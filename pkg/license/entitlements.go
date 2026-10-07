package license

import (
	"context"
	"errors"
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
	enterpriseLimitEntitlementPrefix        = "OBOT_ENTERPRISE_"
	limitEntitlementPrefix                  = "OBOT_"
	userLimitEntitlementUsersSuffix         = "_USERS"
	deviceLimitEntitlementDevicesSuffix     = "_DEVICES"
	auditLogRetentionLimitEntitlementSuffix = "_DAYS_AUDIT_LOG_RETENTION"
)

var (
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

type ProviderEntitlementGate struct {
	licenseProvider *Provider
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

func NewProviderEntitlementGate(licenseProvider *Provider, client kclient.Client) *ProviderEntitlementGate {
	mux := http.NewServeMux()
	for _, path := range entitlementPathsToGate {
		mux.Handle(path, (*fake)(nil))
	}

	return &ProviderEntitlementGate{
		licenseProvider: licenseProvider,
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
// OBOT_ENTERPRISE_<number>_USERS or OBOT_<number>_USERS entitlements define an additive limit.
func (p *Provider) UserLimit(ctx context.Context) (gatewayclient.SystemLimit, error) {
	maximum, unlimited, err := p.resourceLimit(ctx, userLimitEntitlementUsersSuffix, gatewayclient.DefaultUserLimit)
	if err != nil {
		return gatewayclient.SystemLimit{}, err
	}
	return gatewayclient.SystemLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// DeviceLimit returns the maximum number of devices allowed by the current license.
// OBOT_ENTERPRISE grants unlimited devices unless one or more
// OBOT_ENTERPRISE_<number>_DEVICES or OBOT_<number>_DEVICES entitlements define an additive limit.
func (p *Provider) DeviceLimit(ctx context.Context) (gatewayclient.SystemLimit, error) {
	maximum, unlimited, err := p.resourceLimit(ctx, deviceLimitEntitlementDevicesSuffix, gatewayclient.DefaultDeviceLimit)
	if err != nil {
		return gatewayclient.SystemLimit{}, err
	}
	return gatewayclient.SystemLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// AuditLogRetentionLimit returns the number of days MCP and LLM audit logs are kept.
// There's no limit unless one or more OBOT_ENTERPRISE_<number>_DAYS_AUDIT_LOG_RETENTION or
// OBOT_<number>_DAYS_AUDIT_LOG_RETENTION entitlements define an additive limit.
func (p *Provider) AuditLogRetentionLimit(ctx context.Context) (gatewayclient.SystemLimit, error) {
	if err := p.refresh(ctx, false); err != nil {
		return gatewayclient.SystemLimit{}, err
	}

	// Read the limit and whether the license validated under one lock, so a concurrent refresh can't mix the two.
	p.lock.RLock()
	defer p.lock.RUnlock()

	// A license key that didn't validate leaves the limit unknown, not unset, so callers don't mistake it for no limit.
	if p.licenseKeySnapshot.key != "" && p.entitlements == nil {
		return gatewayclient.SystemLimit{}, errors.New("license key is set but not validated")
	}
	maximum, unlimited := p.entitlementLimit(auditLogRetentionLimitEntitlementSuffix, 0)
	return gatewayclient.SystemLimit{
		Maximum:   maximum,
		Unlimited: unlimited,
	}, nil
}

// resourceLimit refreshes the license, then sums the numeric entitlements for the given suffix into a limit.
func (p *Provider) resourceLimit(ctx context.Context, entitlementSuffix string, defaultMaximum int64) (int64, bool, error) {
	if err := p.refresh(ctx, false); err != nil {
		return 0, false, err
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	maximum, unlimited := p.entitlementLimit(entitlementSuffix, defaultMaximum)
	return maximum, unlimited, nil
}

// entitlementLimit sums the numeric entitlements for the given suffix into a limit. The caller holds p.lock.
// A numeric entitlement takes precedence over OBOT_ENTERPRISE, which otherwise means unlimited.
// Without either the limit falls back to defaultMaximum.
func (p *Provider) entitlementLimit(entitlementSuffix string, defaultMaximum int64) (int64, bool) {
	var maximum int64
	var isEnterpriseEdition bool
	for entitlement := range p.entitlements {
		code := string(entitlement)
		if code == EnterpriseEntitlement {
			isEnterpriseEdition = true
			continue
		}

		value, ok := strings.CutPrefix(code, enterpriseLimitEntitlementPrefix)
		if !ok {
			// Didn't find the old enterprise prefix, try the non-enterprise prefix
			value, ok = strings.CutPrefix(code, limitEntitlementPrefix)
			if !ok {
				continue
			}
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

		entitlementMaximum, err := strconv.ParseInt(value, 10, 64)
		if err != nil || entitlementMaximum <= 0 {
			continue
		}

		if entitlementMaximum > math.MaxInt64-maximum {
			maximum = math.MaxInt64
			continue
		}

		maximum += entitlementMaximum
	}

	unlimited := isEnterpriseEdition && maximum == 0
	if maximum == 0 && !unlimited {
		maximum = defaultMaximum
	}

	return maximum, unlimited
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
func (p *Provider) GetLicenseViolations(ctx context.Context, c kclient.Client) ([]Violation, error) {
	violations, err := p.configuredProviderViolations(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("failed to check configured provider license entitlements: %w", err)
	}

	userLimit, err := p.UserLimit(ctx)
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

	deviceLimit, err := p.DeviceLimit(ctx)
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
