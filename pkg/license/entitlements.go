package license

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	enterpriseLimitEntitlementPrefix = "OBOT_ENTERPRISE_"

	// LimitEntitlementPrefix introduces a numeric limit entitlement that is not
	// tied to the enterprise edition, such as OBOT_20_USERS.
	LimitEntitlementPrefix = "OBOT_"

	// UsersEntitlementSuffix ends a user seat limit entitlement.
	UsersEntitlementSuffix = "_USERS"

	// DevicesEntitlementSuffix ends a device limit entitlement.
	DevicesEntitlementSuffix = "_DEVICES"

	// HostedMCPServersEntitlementSuffix ends a hosted MCP server limit entitlement.
	HostedMCPServersEntitlementSuffix = "_HOSTED_MCP_SERVERS"

	// AuditLogDaysEntitlementSuffix ends an audit log retention entitlement.
	AuditLogDaysEntitlementSuffix = "_AUDIT_LOG_DAYS"
)

var (
	// limitEntitlementPrefixes are tried in order, so OBOT_ENTERPRISE_10_USERS
	// is read as an enterprise limit rather than as the malformed bare limit
	// "ENTERPRISE_10".
	limitEntitlementPrefixes = []string{
		enterpriseLimitEntitlementPrefix,
		LimitEntitlementPrefix,
	}

	entitlementPathsToGate = []string{
		"/mcp-connect/{mcp_id}",
		"/mcp-connect/{mcp_id}/",
		"GET /oauth/authorize",
		"GET /oauth/authorize/",
		"GET /oauth/consent/",
		"POST /oauth/consent/",
		"GET /oauth/complete/",
		"GET /oauth/mcp/callback/",
		"POST /oauth/",
		"PUT /oauth/",
		"GET /api/oauth/vmcp/{mcp_id}",
		"GET /api/oauth/vmcp/{mcp_id}/",
		"/api/llm-proxy/",
		"/api/skills",
		"/api/skills/",
		"POST /api/devices/scans",
	}
)

// Violation describes a configured provider that requires license entitlements
// that are not currently available.
type Violation struct {
	Type                 string   `json:"type"`
	Namespace            string   `json:"namespace"`
	Name                 string   `json:"name"`
	RequiredEntitlements []string `json:"requiredEntitlements"`
	MissingEntitlements  []string `json:"missingEntitlements"`
	Message              string   `json:"message"`
}

// LimitProvider resolves the resource limits an installation is entitled to.
type LimitProvider interface {
	UserLimit(context.Context) (gatewayclient.UserLimit, error)
	DeviceLimit(context.Context) (gatewayclient.DeviceLimit, error)
	HostedMCPServerLimit(context.Context) (gatewayclient.HostedMCPServerLimit, error)
	AuditLogRetention(context.Context) (gatewayclient.AuditLogRetention, error)
}

type ProviderEntitlementGate struct {
	licenseProvider *Provider
	restrictor      *Restrictor
	client          kclient.Client
	mux             *http.ServeMux
}

// fake is a fake handler that does fake things
type fake struct{}

// GetDistributionFromEntitlements returns the product distribution represented by the entitlements.
func GetDistributionFromEntitlements(entitlements []string) types.ProductTelemetryDistribution {
	switch {
	case slices.Contains(entitlements, CloudEntitlement):
		return types.ProductTelemetryDistributionCloud
	case slices.Contains(entitlements, EnterpriseEntitlement):
		return types.ProductTelemetryDistributionEnterprise
	case slices.Contains(entitlements, CommunityEntitlement):
		return types.ProductTelemetryDistributionRegistered
	default:
		return types.ProductTelemetryDistributionUnregistered
	}
}

func NewProviderEntitlementGate(licenseProvider *Provider, restrictor *Restrictor, client kclient.Client) *ProviderEntitlementGate {
	mux := http.NewServeMux()
	for _, path := range entitlementPathsToGate {
		mux.Handle(path, (*fake)(nil))
	}

	return &ProviderEntitlementGate{
		licenseProvider: licenseProvider,
		restrictor:      restrictor,
		client:          client,
		mux:             mux,
	}
}

func (g *ProviderEntitlementGate) Check(req *http.Request) error {
	if g == nil || !g.requiresProviderEntitlements(req) {
		return nil
	}

	violations, err := g.licenseProvider.configuredProviderViolations(req.Context(), g.client)
	if err != nil {
		return fmt.Errorf("failed to check provider license entitlements: %w", err)
	}
	if len(violations) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired, "configured provider is missing required license entitlements")
	}

	restricted, err := g.restrictor.Restricted(req.Context())
	if err != nil {
		return fmt.Errorf("failed to check resource limits: %w", err)
	}
	if restricted {
		return types.NewErrHTTP(http.StatusPaymentRequired, "this installation is using more than it is licensed for; ask your administrator to reduce usage or raise the limits")
	}

	return nil
}

func (g *ProviderEntitlementGate) requiresProviderEntitlements(req *http.Request) bool {
	_, pattern := g.mux.Handler(req)
	return pattern != ""
}

// MissingEntitlements returns the required entitlements that are unavailable
// from the current database/config license key.
func (p *Provider) MissingEntitlements(ctx context.Context, requiredEntitlements []string) ([]string, error) {
	if err := p.refresh(ctx, false); err != nil {
		return nil, err
	}
	return p.missingEntitlements(requiredEntitlements), nil
}

func (p *Provider) missingEntitlements(requiredEntitlements []string) []string {
	var missing []string
	for _, entitlement := range requiredEntitlements {
		if !p.hasEntitlement(entitlement) {
			missing = append(missing, entitlement)
		}
	}
	return missing
}

// UserLimit returns the maximum number of users allowed by the current license.
// OBOT_ENTERPRISE grants unlimited users unless one or more
// OBOT_ENTERPRISE_<number>_USERS entitlements define an additive limit.
func (p *Provider) UserLimit(ctx context.Context) (gatewayclient.UserLimit, error) {
	maximum, unlimited, err := p.resourceLimit(ctx, UsersEntitlementSuffix, gatewayclient.DefaultUserLimit)
	if err != nil {
		return gatewayclient.UserLimit{}, err
	}
	return gatewayclient.UserLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// DeviceLimit returns the maximum number of devices allowed by the current license.
// OBOT_ENTERPRISE grants unlimited devices unless one or more
// OBOT_ENTERPRISE_<number>_DEVICES entitlements define an additive limit.
func (p *Provider) DeviceLimit(ctx context.Context) (gatewayclient.DeviceLimit, error) {
	maximum, unlimited, err := p.resourceLimit(ctx, DevicesEntitlementSuffix, gatewayclient.DefaultDeviceLimit)
	if err != nil {
		return gatewayclient.DeviceLimit{}, err
	}
	return gatewayclient.DeviceLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// HostedMCPServerLimit returns the maximum number of hosted MCP servers allowed by
// the current license. OBOT_ENTERPRISE grants unlimited hosted servers unless one
// or more OBOT_<number>_HOSTED_MCP_SERVERS entitlements define an additive limit.
func (p *Provider) HostedMCPServerLimit(ctx context.Context) (gatewayclient.HostedMCPServerLimit, error) {
	maximum, unlimited, err := p.resourceLimit(ctx, HostedMCPServersEntitlementSuffix, gatewayclient.DefaultHostedMCPServerLimit)
	if err != nil {
		return gatewayclient.HostedMCPServerLimit{}, err
	}
	return gatewayclient.HostedMCPServerLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// AuditLogRetention returns how long the current license allows MCP and LLM audit
// logs to be kept. No entitlement leaves the retention undefined, so the server's
// configured retention applies.
func (p *Provider) AuditLogRetention(ctx context.Context) (gatewayclient.AuditLogRetention, error) {
	days, unlimited, err := p.resourceLimit(ctx, AuditLogDaysEntitlementSuffix, gatewayclient.DefaultAuditLogRetentionDays)
	if err != nil {
		return gatewayclient.AuditLogRetention{}, err
	}
	return gatewayclient.AuditLogRetention{
		Days:      days,
		Unlimited: unlimited,
	}, nil
}

// ResourceLimit resolves a numeric resource limit from a set of entitlement names.
//
// A limit is spelled OBOT_<number><suffix> or OBOT_ENTERPRISE_<number><suffix>;
// matching entitlements are summed. Names whose middle segment is not a number are
// ignored. OBOT_ENTERPRISE on its own makes the limit unlimited, unless a numeric
// entitlement defines one. When nothing defines a limit, defaultMaximum applies.
func ResourceLimit(entitlements []string, entitlementSuffix string, defaultMaximum int64) (int64, bool) {
	var (
		maximum             int64
		isEnterpriseEdition bool
	)
	for _, code := range entitlements {
		if code == EnterpriseEntitlement {
			isEnterpriseEdition = true
			continue
		}

		value, ok := entitlementLimit(code, entitlementSuffix)
		if !ok {
			continue
		}

		if value > math.MaxInt64-maximum {
			maximum = math.MaxInt64
		} else {
			maximum += value
		}
	}

	unlimited := isEnterpriseEdition && maximum == 0
	if maximum == 0 && !unlimited {
		maximum = defaultMaximum
	}

	return maximum, unlimited
}

// entitlementLimit returns the positive number an entitlement name encodes for the
// given suffix.
func entitlementLimit(code, entitlementSuffix string) (int64, bool) {
	for _, prefix := range limitEntitlementPrefixes {
		value, ok := strings.CutPrefix(code, prefix)
		if !ok {
			continue
		}
		value, ok = strings.CutSuffix(value, entitlementSuffix)
		if !ok {
			continue
		}
		if value == "" || strings.IndexFunc(value, func(r rune) bool {
			return r < '0' || r > '9'
		}) >= 0 {
			continue
		}

		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit <= 0 {
			continue
		}
		return limit, true
	}
	return 0, false
}

func (p *Provider) resourceLimit(ctx context.Context, entitlementSuffix string, defaultMaximum int64) (int64, bool, error) {
	if err := p.refresh(ctx, false); err != nil {
		return 0, false, err
	}

	p.lock.RLock()
	entitlements := make([]string, 0, len(p.entitlements))
	for entitlement := range p.entitlements {
		entitlements = append(entitlements, string(entitlement))
	}
	p.lock.RUnlock()

	maximum, unlimited := ResourceLimit(entitlements, entitlementSuffix, defaultMaximum)
	return maximum, unlimited, nil
}

// RequireEntitlements returns Payment Required if any required entitlements are unavailable.
func (p *Provider) RequireEntitlements(ctx context.Context, requiredEntitlements []string) error {
	missing, err := p.MissingEntitlements(ctx, requiredEntitlements)
	if err != nil {
		return fmt.Errorf("failed to refresh license entitlements: %w", err)
	}
	if len(missing) == 0 {
		return nil
	}
	return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("missing required license entitlements: %v", missing))
}

// GetLicenseViolations returns all license violations for the configured auth/model providers and resource limits.
func (p *Provider) GetLicenseViolations(ctx context.Context, c kclient.Client, limits LimitProvider) ([]Violation, error) {
	violations, err := p.configuredProviderViolations(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to check configured provider license entitlements: %w", err)
	}

	userLimit, err := limits.UserLimit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check user limit: %w", err)
	}
	if !userLimit.Unlimited {
		userCount, err := p.gatewayClient.UserCount(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check user count: %w", err)
		}

		if userCount > userLimit.Maximum {
			violations = append(violations, Violation{
				Type:    "userLimit",
				Message: fmt.Sprintf("user count (%d) exceeds maximum limit (%d)", userCount, userLimit.Maximum),
			})
		}
	}

	deviceLimit, err := limits.DeviceLimit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check device limit: %w", err)
	}
	if !deviceLimit.Unlimited {
		deviceCount, err := p.gatewayClient.DeviceCount(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to check device count: %w", err)
		}

		if deviceCount > deviceLimit.Maximum {
			violations = append(violations, Violation{
				Type:    "deviceLimit",
				Message: fmt.Sprintf("device count (%d) exceeds maximum limit (%d)", deviceCount, deviceLimit.Maximum),
			})
		}
	}

	return violations, nil
}

// configuredProviderViolations returns any globally configured auth/model providers
// that are currently missing required license entitlements.
func (p *Provider) configuredProviderViolations(ctx context.Context, c kclient.Client) ([]Violation, error) {
	if err := p.refresh(ctx, false); err != nil {
		return nil, fmt.Errorf("failed to refresh license entitlements: %w", err)
	}

	modelProviderViolations, err := p.configuredModelProviderViolations(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to check model provider license entitlements: %w", err)
	}

	authProviderViolations, err := p.configuredAuthProviderViolations(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to check auth provider license entitlements: %w", err)
	}

	return append(modelProviderViolations, authProviderViolations...), nil
}

func (p *Provider) configuredModelProviderViolations(ctx context.Context, c kclient.Client) ([]Violation, error) {
	var modelProviders v1.ModelProviderList
	if err := c.List(ctx, &modelProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list model providers: %w", err)
	}

	var violations []Violation
	for _, mp := range modelProviders.Items {
		if mp.Status.Configured {
			missingEntitlements := p.missingEntitlements(mp.Spec.RequiredEntitlements)
			if len(missingEntitlements) > 0 {
				violations = append(violations, Violation{
					Type:                 "modelProvider",
					Namespace:            mp.Namespace,
					Name:                 mp.Name,
					RequiredEntitlements: mp.Spec.RequiredEntitlements,
					MissingEntitlements:  missingEntitlements,
					Message:              "missing required entitlements",
				})
			}
		}
	}

	return violations, nil
}

func (p *Provider) configuredAuthProviderViolations(ctx context.Context, c kclient.Client) ([]Violation, error) {
	var authProviders v1.AuthProviderList
	if err := c.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list auth providers: %w", err)
	}

	var violations []Violation
	for _, ap := range authProviders.Items {
		if ap.Status.Configured {
			missingEntitlements := p.missingEntitlements(ap.Spec.RequiredEntitlements)
			if len(missingEntitlements) > 0 {
				violations = append(violations, Violation{
					Type:                 "authProvider",
					Namespace:            ap.Namespace,
					Name:                 ap.Name,
					RequiredEntitlements: ap.Spec.RequiredEntitlements,
					MissingEntitlements:  missingEntitlements,
				})
			}
		}
	}

	return violations, nil
}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
