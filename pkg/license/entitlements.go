package license

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var entitlementPathsToGate = []string{
	"/mcp-connect/{mcp_id}",
	"/mcp-connect/{mcp_id}/",
	"GET /oauth/authorize",
	"GET /oauth/authorize/",
	"GET /oauth/consent/",
	"POST /oauth/consent/",
	"GET /oauth/mcp/callback/",
	"POST /oauth/",
	"PUT /oauth/",
	"GET /api/oauth/composite/{mcp_id}",
	"/api/llm-proxy/",
	"/api/skills",
	"/api/skills/",
	"POST /api/devices/scans",
}

// ProviderViolation describes a configured provider that requires license entitlements
// that are not currently available.
type ProviderViolation struct {
	Type                 string   `json:"type"`
	Namespace            string   `json:"namespace"`
	Name                 string   `json:"name"`
	RequiredEntitlements []string `json:"requiredEntitlements"`
	MissingEntitlements  []string `json:"missingEntitlements"`
}

type ProviderMeta struct {
	RequiredEntitlements []string                               `json:"requiredEntitlements"`
	EnvVars              []types.ProviderConfigurationParameter `json:"envVars"`
}

type ProviderEntitlementGate struct {
	licenseProvider *KeygenProvider
	client          kclient.Client
	mux             *http.ServeMux
}

func NewProviderEntitlementGate(licenseProvider *KeygenProvider, client kclient.Client) *ProviderEntitlementGate {
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

	violations, err := g.licenseProvider.ConfiguredProviderViolations(req.Context(), g.client)
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

// Missing returns the required entitlements that are unavailable from the current license.
func (p *KeygenProvider) MissingEntitlements(requiredEntitlements []string) []string {
	var missing []string
	for _, entitlement := range requiredEntitlements {
		if !p.hasEntitlement(entitlement) {
			missing = append(missing, entitlement)
		}
	}
	return missing
}

// Require returns Payment Required if any required entitlements are unavailable.
func (p *KeygenProvider) RequireEntitlements(requiredEntitlements []string) error {
	missing := p.MissingEntitlements(requiredEntitlements)
	if len(missing) == 0 {
		return nil
	}
	return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("missing required license entitlements: %v", missing))
}

// ConfiguredProviderViolations returns any globally configured auth/model providers
// that are currently missing required license entitlements.
func (p *KeygenProvider) ConfiguredProviderViolations(ctx context.Context, c kclient.Client) ([]ProviderViolation, error) {
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

func (p *KeygenProvider) configuredModelProviderViolations(ctx context.Context, c kclient.Client) ([]ProviderViolation, error) {
	var modelProviders v1.ModelProviderList
	if err := c.List(ctx, &modelProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list model providers: %w", err)
	}

	var violations []ProviderViolation
	for _, mp := range modelProviders.Items {
		if mp.Status.Configured {
			missingEntitlements := p.MissingEntitlements(mp.Spec.RequiredEntitlements)
			if len(missingEntitlements) > 0 {
				violations = append(violations, ProviderViolation{
					Type:                 "modelProvider",
					Namespace:            mp.Namespace,
					Name:                 mp.Name,
					RequiredEntitlements: mp.Spec.RequiredEntitlements,
					MissingEntitlements:  missingEntitlements,
				})
			}
		}
	}

	return violations, nil
}

func (p *KeygenProvider) configuredAuthProviderViolations(ctx context.Context, c kclient.Client) ([]ProviderViolation, error) {
	var authProviders v1.AuthProviderList
	if err := c.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list auth providers: %w", err)
	}

	var violations []ProviderViolation
	for _, ap := range authProviders.Items {
		if ap.Status.Configured {
			missingEntitlements := p.MissingEntitlements(ap.Spec.RequiredEntitlements)
			if len(missingEntitlements) > 0 {
				violations = append(violations, ProviderViolation{
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
