package handlers

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	"github.com/obot-platform/obot/pkg/upgrade"
	"github.com/obot-platform/obot/pkg/version"
)

const (
	SessionStoreDB     SessionStore = "db"
	SessionStoreCookie SessionStore = "cookie"
)

type SessionStore string

type UpgradeStatusReader interface {
	Status() upgrade.Status
}

type VersionHandlerOptions struct {
	GatewayClient           *client.Client
	StorageClient           storage.Client
	LicenseProvider         *license.Provider
	PostgresDSN             string
	Engine                  string
	MCPNetworkPolicyEnabled bool
	MCPDefaultDenyAllEgress bool
	AuthEnabled             bool
	MessagePoliciesEnabled  bool
	AgentsEnabled           bool
	HostedAgentsEnabled     bool
	HideK8sDetails          bool
	UpgradeStatusReader     UpgradeStatusReader
}

type VersionHandler struct {
	VersionHandlerOptions

	sessionStore SessionStore
}

func sessionStoreFromPostgresDSN(postgresDSN string) SessionStore {
	if postgresDSN != "" {
		return SessionStoreDB
	}
	return SessionStoreCookie
}

func NewVersionHandler(opts VersionHandlerOptions) *VersionHandler {
	return &VersionHandler{
		VersionHandlerOptions: opts,
		sessionStore:          sessionStoreFromPostgresDSN(opts.PostgresDSN),
	}
}

func (v *VersionHandler) GetVersion(req api.Context) error {
	response, err := v.getVersionResponse(req.Context())
	if err != nil {
		return err
	}
	return req.Write(response)
}

func (v *VersionHandler) getVersionResponse(ctx context.Context) (map[string]any, error) {
	engine := v.Engine
	if mcp.IsKubernetesBackend(engine) {
		engine = mcp.RuntimeBackendKubernetes
	}

	violations, err := v.LicenseProvider.GetLicenseViolations(ctx, v.StorageClient)
	if err != nil {
		return nil, err
	}

	upgradeStatus := v.upgradeStatus()

	entitlements, err := v.LicenseProvider.Entitlements(ctx)
	if err != nil {
		return nil, err
	}

	userCount, err := v.GatewayClient.UserCount(ctx)
	if err != nil {
		return nil, err
	}

	deviceCount, err := v.GatewayClient.DeviceCount(ctx)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"upgradeAvailable":             upgradeStatus.UpgradeAvailable,
		"latestVersion":                upgradeStatus.LatestVersion,
		"obot":                         version.Get().String(),
		"authEnabled":                  v.AuthEnabled,
		"sessionStore":                 v.sessionStore,
		"enterprise":                   slices.Contains(entitlements, license.EnterpriseEntitlement),
		"community":                    slices.Contains(entitlements, license.CommunityEntitlement),
		"licenseEntitlements":          entitlements,
		"userCount":                    userCount,
		"deviceCount":                  deviceCount,
		"engine":                       engine,
		"mcpNetworkPolicyEnabled":      v.MCPNetworkPolicyEnabled,
		"mcpDefaultDenyAllEgress":      v.MCPDefaultDenyAllEgress,
		"licenseEntitlementViolations": violations,
		"missingLicenseEntitlements":   missingEntitlements(violations),
	}
	for key, value := range v.featureValues() {
		values[key] = value
	}

	userLimit, err := v.LicenseProvider.UserLimit(ctx)
	if err != nil {
		return nil, err
	}
	if !userLimit.Unlimited {
		values["userLimit"] = userLimit.Maximum
	}

	deviceLimit, err := v.LicenseProvider.DeviceLimit(ctx)
	if err != nil {
		return nil, err
	}
	if !deviceLimit.Unlimited {
		values["deviceLimit"] = deviceLimit.Maximum
	}

	if versions := os.Getenv("OBOT_SERVER_VERSIONS"); versions != "" {
		for pair := range strings.SplitSeq(versions, ",") {
			key, value, ok := strings.Cut(pair, "=")
			if !ok {
				continue
			}
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}

	return values, nil
}

func (v *VersionHandler) upgradeStatus() upgrade.Status {
	if v.UpgradeStatusReader == nil {
		return upgrade.Status{}
	}
	return v.UpgradeStatusReader.Status()
}

func (v *VersionHandler) featureValues() map[string]bool {
	return map[string]bool{
		"messagePoliciesEnabled": v.MessagePoliciesEnabled,
		"agentsEnabled":          v.AgentsEnabled,
		"hostedAgentsEnabled":    v.HostedAgentsEnabled,
		"hideK8sDetails":         v.HideK8sDetails,
	}
}

func missingEntitlements(violations []license.Violation) []string {
	seen := make(map[string]struct{})
	for _, violation := range violations {
		for _, entitlement := range violation.MissingEntitlements {
			seen[entitlement] = struct{}{}
		}
	}
	missing := make([]string, 0, len(seen))
	for entitlement := range seen {
		missing = append(missing, entitlement)
	}
	slices.Sort(missing)
	return missing
}
