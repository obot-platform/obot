package producttelemetry

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/storage"
	storagev1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/upgrade"
	"github.com/obot-platform/obot/pkg/version"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type requestGateway struct {
	lock           sync.Mutex
	properties     map[string]string
	propertyErrors map[string]error
	propertyCalls  int
	metricErr      error
	totalUsers     int64
	activeUsers    int
	mcpToolCalls   int64
	llmAuditLogs   int64
	deviceScans    int64
	enforcements   int64
	dayStart       time.Time
	dayEnd         time.Time
}

type entitlementProviderFunc func(context.Context) ([]string, error)

type errorStorageReader struct {
	err error
}

func (e errorStorageReader) Get(context.Context, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
	return e.err
}

func (e errorStorageReader) List(context.Context, kclient.ObjectList, ...kclient.ListOption) error {
	return e.err
}

func newRequestGateway() *requestGateway {
	return &requestGateway{
		properties: map[string]string{
			upgrade.InstallationIDPropertyKey:   "installation-id",
			license.LicenseMachineIDPropertyKey: "machine-id",
		},
		totalUsers:   42,
		activeUsers:  3,
		mcpToolCalls: 7,
		llmAuditLogs: 8,
		deviceScans:  9,
		enforcements: 10,
	}
}

func (g *requestGateway) GetOrCreateProperty(_ context.Context, key, value string) (gatewaytypes.Property, error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.propertyCalls++
	if err := g.propertyErrors[key]; err != nil {
		return gatewaytypes.Property{}, err
	}
	if g.properties == nil {
		g.properties = map[string]string{}
	}
	if g.properties[key] == "" {
		g.properties[key] = value
	}
	return gatewaytypes.Property{Key: key, Value: g.properties[key]}, nil
}

func (g *requestGateway) UserCount(context.Context) (int64, error) {
	return g.totalUsers, g.metricErr
}

func (g *requestGateway) ActiveUserCountByDate(_ context.Context, start, end time.Time) (int64, error) {
	g.recordWindow(start, end)
	return int64(g.activeUsers), g.metricErr
}

func (g *requestGateway) MCPToolCallCount(_ context.Context, start, end time.Time) (int64, error) {
	g.recordWindow(start, end)
	return g.mcpToolCalls, g.metricErr
}

func (g *requestGateway) LLMAuditLogCount(_ context.Context, start, end time.Time) (int64, error) {
	g.recordWindow(start, end)
	return g.llmAuditLogs, g.metricErr
}

func (g *requestGateway) DeviceScanCount(_ context.Context, start, end time.Time) (int64, error) {
	g.recordWindow(start, end)
	return g.deviceScans, g.metricErr
}

func (g *requestGateway) EnforcementDecisionCount(_ context.Context, start, end time.Time) (int64, error) {
	g.recordWindow(start, end)
	return g.enforcements, g.metricErr
}

func (g *requestGateway) recordWindow(start, end time.Time) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.dayStart.IsZero() {
		g.dayStart = start
		g.dayEnd = end
	}
}

func (f entitlementProviderFunc) Entitlements(ctx context.Context) ([]string, error) {
	return f(ctx)
}

func testEntitlements(values ...string) licenseEntitlementProvider {
	return entitlementProviderFunc(func(context.Context) ([]string, error) { return values, nil })
}

func testStorageClient(objects ...kclient.Object) storage.Client {
	return storage.Client(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).Build())
}

func TestBuildRequestPopulatesAllFields(t *testing.T) {
	gateway := newRequestGateway()
	builtInEntry := &storagev1.MCPServerCatalogEntry{
		Name: "default-github", Namespace: system.DefaultNamespace,
		Spec: storagev1.MCPServerCatalogEntrySpec{
			SourceURL: "/built-ins",
			Manifest:  clienttypes.MCPServerCatalogEntryManifest{EntryKey: "github", Name: "GitHub"},
		},
		Status: storagev1.MCPServerCatalogEntryStatus{UserCount: 7},
	}
	customEntry := &storagev1.MCPServerCatalogEntry{
		Name: "custom", Namespace: system.DefaultNamespace,
		Spec: storagev1.MCPServerCatalogEntrySpec{
			Manifest: clienttypes.MCPServerCatalogEntryManifest{Name: "Custom"},
		},
	}
	builtInServer := &storagev1.MCPServer{
		Name: "github-1", Namespace: system.DefaultNamespace,
		Spec: storagev1.MCPServerSpec{MCPServerCatalogEntryName: "default-github"},
	}
	customServer := &storagev1.MCPServer{
		Name: "custom-1", Namespace: system.DefaultNamespace,
	}
	templateServer := &storagev1.MCPServer{
		Name: "template", Namespace: system.DefaultNamespace,
		Spec: storagev1.MCPServerSpec{Template: true},
	}
	authProvider := &storagev1.AuthProvider{
		Name: "github", Namespace: system.DefaultNamespace,
		Status: storagev1.AuthProviderStatus{Configured: true},
	}
	skill1 := &storagev1.Skill{Name: "skill-1", Namespace: system.DefaultNamespace}
	skill2 := &storagev1.Skill{Name: "skill-2", Namespace: system.DefaultNamespace}
	storageClient := testStorageClient(
		builtInEntry, customEntry, builtInServer, customServer, templateServer, authProvider, skill1, skill2,
	)

	before := time.Now().UTC()
	report, err := buildRequest(t.Context(), gateway, storageClient, testEntitlements(license.CommunityEntitlement), "/built-ins", "k8s")
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	after := time.Now().UTC()

	if report.InstallationID != "installation-id" || report.LicenseMachineID != "machine-id" {
		t.Fatalf("installation identity = %q/%q", report.InstallationID, report.LicenseMachineID)
	}
	if report.ReportedAt.Before(before) || report.ReportedAt.After(after) || report.ReportedAt.Location() != time.UTC {
		t.Fatalf("reportedAt = %v, want UTC time between %v and %v", report.ReportedAt, before, after)
	}
	if report.Distribution != clienttypes.ProductTelemetryDistributionRegistered || report.Engine != "kubernetes" || report.CurrentVersion != version.Get().String() {
		t.Fatalf("metadata = distribution:%q engine:%q version:%q", report.Distribution, report.Engine, report.CurrentVersion)
	}

	metrics := report.Metrics
	if metrics == nil {
		t.Fatal("metrics are nil")
	}
	assertInt64(t, "total users", metrics.TotalUsers, 42)
	assertInt64(t, "active users", metrics.ActiveUsers, 3)
	assertInt64(t, "deployed MCP servers", metrics.DeployedMCPServers, 2)
	assertInt64(t, "custom MCP entries", metrics.CustomMCPServerEntryCount, 1)
	assertInt64(t, "MCP tool calls", metrics.MCPToolCallCount, 7)
	assertInt64(t, "LLM audit logs", metrics.LLMAuditLogCount, 8)
	assertInt64(t, "Sentry scans", metrics.SentryScanCount, 9)
	assertInt64(t, "Sentry enforcement events", metrics.SentryEnforcementEventCount, 10)
	assertInt64(t, "managed skills", metrics.ManagedSkillCount, 2)
	if metrics.AuthProviderType == nil || *metrics.AuthProviderType != "github" {
		t.Fatalf("auth provider type = %v, want github", metrics.AuthProviderType)
	}
	if metrics.BuiltInMCPServers == nil || len(*metrics.BuiltInMCPServers) != 1 {
		t.Fatalf("built-in MCP servers = %#v, want one", metrics.BuiltInMCPServers)
	}
	if got := (*metrics.BuiltInMCPServers)[0]; got.ID != "github" || got.Name != "GitHub" || got.DeploymentCount != 1 || got.UserCount != 7 {
		t.Fatalf("built-in MCP server = %#v", got)
	}

	wantEnd := report.ReportedAt.Truncate(24 * time.Hour)
	if !gateway.dayEnd.Equal(wantEnd) || !gateway.dayStart.Equal(wantEnd.Add(-24*time.Hour)) {
		t.Fatalf("metric window = [%v,%v), want previous full UTC day ending %v", gateway.dayStart, gateway.dayEnd, wantEnd)
	}
}

func TestBuildRequestPreservesUnavailableMetrics(t *testing.T) {
	gateway := newRequestGateway()
	gateway.metricErr = errors.New("metrics unavailable")
	report, err := buildRequest(t.Context(), gateway, errorStorageReader{err: errors.New("storage unavailable")}, testEntitlements(), "", "docker")
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if report.Metrics.TotalUsers != nil || report.Metrics.ActiveUsers != nil || report.Metrics.MCPToolCallCount != nil ||
		report.Metrics.LLMAuditLogCount != nil || report.Metrics.SentryScanCount != nil || report.Metrics.SentryEnforcementEventCount != nil ||
		report.Metrics.DeployedMCPServers != nil || report.Metrics.CustomMCPServerEntryCount != nil || report.Metrics.BuiltInMCPServers != nil ||
		report.Metrics.AuthProviderType != nil || report.Metrics.ManagedSkillCount != nil {
		t.Fatalf("unavailable metrics = %#v, want nil fields", report.Metrics)
	}

	zeroGateway := newRequestGateway()
	zeroGateway.totalUsers = 0
	zeroGateway.activeUsers = 0
	zeroGateway.mcpToolCalls = 0
	zeroGateway.llmAuditLogs = 0
	zeroGateway.deviceScans = 0
	zeroGateway.enforcements = 0
	report, err = buildRequest(t.Context(), zeroGateway, testStorageClient(), testEntitlements(), "", "docker")
	if err != nil {
		t.Fatalf("buildRequest() measured zero error = %v", err)
	}
	assertInt64(t, "measured zero total users", report.Metrics.TotalUsers, 0)
	assertInt64(t, "measured zero active users", report.Metrics.ActiveUsers, 0)
	assertInt64(t, "measured zero deployed MCP servers", report.Metrics.DeployedMCPServers, 0)
	assertInt64(t, "measured zero custom MCP entries", report.Metrics.CustomMCPServerEntryCount, 0)
	assertInt64(t, "measured zero MCP tool calls", report.Metrics.MCPToolCallCount, 0)
	assertInt64(t, "measured zero LLM audit logs", report.Metrics.LLMAuditLogCount, 0)
	assertInt64(t, "measured zero Sentry scans", report.Metrics.SentryScanCount, 0)
	assertInt64(t, "measured zero Sentry enforcement events", report.Metrics.SentryEnforcementEventCount, 0)
	assertInt64(t, "measured zero managed skills", report.Metrics.ManagedSkillCount, 0)
	if report.Metrics.BuiltInMCPServers == nil || len(*report.Metrics.BuiltInMCPServers) != 0 {
		t.Fatalf("measured zero built-ins = %#v, want empty slice", report.Metrics.BuiltInMCPServers)
	}
	if report.Metrics.AuthProviderType == nil || *report.Metrics.AuthProviderType != "" {
		t.Fatalf("measured zero auth provider = %#v, want empty string", report.Metrics.AuthProviderType)
	}
}

func TestBuildRequestPreservesUnavailableDistribution(t *testing.T) {
	report, err := buildRequest(
		t.Context(),
		newRequestGateway(),
		testStorageClient(),
		entitlementProviderFunc(func(context.Context) ([]string, error) {
			return nil, errors.New("license unavailable")
		}),
		"",
		"docker",
	)
	if err != nil {
		t.Fatalf("buildRequest() error = %v", err)
	}
	if report.Distribution != clienttypes.ProductTelemetryDistributionUnregistered {
		t.Fatalf("distribution = %q, want %q", report.Distribution, clienttypes.ProductTelemetryDistributionUnregistered)
	}
}

func TestBuildRequestIdentityFailures(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr string
	}{
		{name: "installation ID", key: upgrade.InstallationIDPropertyKey, wantErr: "get installation ID"},
		{name: "license machine ID", key: license.LicenseMachineIDPropertyKey, wantErr: "get license machine ID"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gateway := newRequestGateway()
			gateway.propertyErrors = map[string]error{testCase.key: errors.New("database unavailable")}
			_, err := buildRequest(t.Context(), gateway, testStorageClient(), testEntitlements(), "", "docker")
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("buildRequest() error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func assertInt64(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", name, got, want)
	}
}
