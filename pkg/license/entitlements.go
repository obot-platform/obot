package license

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	enterpriseUserLimitEntitlementPrefix = "OBOT_ENTERPRISE_"
	userLimitEntitlementUsersSuffix      = "_USERS"
)

var entitlementPathsToGate = []string{
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
	"GET /api/oauth/composite/{mcp_id}",
	"/api/llm-proxy/",
	"/api/skills",
	"/api/skills/",
	"POST /api/devices/scans",
}

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

type ProviderMeta struct {
	RequiredEntitlements []string                               `json:"requiredEntitlements"`
	EnvVars              []types.ProviderConfigurationParameter `json:"envVars"`
}

type ProviderEntitlementGate struct {
	licenseProvider *Provider
	client          kclient.Client
	mux             *http.ServeMux
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
// OBOT_ENTERPRISE_AUTH_PROVIDERS grants unlimited users unless one or more
// OBOT_ENTERPRISE_<number>_USERS entitlements define an additive limit.
func (p *Provider) UserLimit(ctx context.Context) (gatewayclient.UserLimit, error) {
	if err := p.refresh(ctx, false); err != nil {
		return gatewayclient.UserLimit{}, err
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	limit := gatewayclient.UserLimit{}
	var isEnterpriseEdition bool
	for entitlement := range p.entitlements {
		code := string(entitlement)
		if code == EnterpriseEntitlement {
			isEnterpriseEdition = true
			continue
		}

		value, ok := strings.CutPrefix(code, enterpriseUserLimitEntitlementPrefix)
		if !ok {
			continue
		}
		value, ok = strings.CutSuffix(value, userLimitEntitlementUsersSuffix)
		if !ok {
			continue
		}
		if value == "" || strings.IndexFunc(value, func(r rune) bool {
			return r < '0' || r > '9'
		}) >= 0 {
			continue
		}

		maximum, err := strconv.ParseInt(value, 10, 64)
		if err != nil || maximum <= 0 {
			continue
		}

		if maximum > math.MaxInt64-limit.Maximum {
			limit.Maximum = math.MaxInt64
		} else {
			limit.Maximum += maximum
		}
	}

	limit.Unlimited = isEnterpriseEdition && limit.Maximum == 0
	if limit.Maximum == 0 && !limit.Unlimited {
		limit.Maximum = gatewayclient.DefaultUserLimit
	}

	return limit, nil
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

// GetLicenseViolations returns all license violations for the configured auth/model providers and user limits.
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

// fake is a fake handler that does fake things
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
