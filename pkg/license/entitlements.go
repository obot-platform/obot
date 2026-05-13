package license

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var entitlementPathsToGate = []string{
	"/mcp-connect/{mcp_id}",
	"/mcp-connect/{mcp_id}/",
	"/oauth/",
	"GET /api/oauth/composite/{mcp_id}",
	"/api/llm-proxy/",
	"/api/skills",
	"/api/skills/",
	"POST /api/devices/scans",
}

// ProviderViolation describes a configured provider that requires license entitlements
// that are not currently available.
type ProviderViolation struct {
	Type                 v1.ToolReferenceType `json:"type"`
	Namespace            string               `json:"namespace"`
	Name                 string               `json:"name"`
	RequiredEntitlements []string             `json:"requiredEntitlements"`
	MissingEntitlements  []string             `json:"missingEntitlements"`
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
func (p *KeygenProvider) Missing(requiredEntitlements []string) []string {
	var missing []string
	for _, entitlement := range requiredEntitlements {
		if !p.hasEntitlement(entitlement) {
			missing = append(missing, entitlement)
		}
	}
	return missing
}

// Require returns Payment Required if any required entitlements are unavailable.
func (p *KeygenProvider) Require(requiredEntitlements []string) error {
	missing := p.Missing(requiredEntitlements)
	if len(missing) == 0 {
		return nil
	}
	return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("missing required license entitlements: %v", missing))
}

// RequireForProvider returns Payment Required if the provider's metadata requires
// entitlements that are not currently available.
func (p *KeygenProvider) RequireForProvider(toolRef v1.ToolReference) error {
	meta, err := MetaForProvider(toolRef)
	if err != nil {
		return err
	}
	return p.Require(meta.RequiredEntitlements)
}

// ConfiguredProviderViolations returns any globally configured auth/model providers
// that are currently missing required license entitlements.
func (p *KeygenProvider) ConfiguredProviderViolations(ctx context.Context, c kclient.Client) ([]ProviderViolation, error) {
	var violations []ProviderViolation
	for _, providerType := range []v1.ToolReferenceType{v1.ToolReferenceTypeAuthProvider, v1.ToolReferenceTypeModelProvider} {
		providerViolations, err := p.configuredProviderViolationsForType(ctx, c, providerType)
		if err != nil {
			return nil, err
		}
		violations = append(violations, providerViolations...)
	}
	return violations, nil
}

func (p *KeygenProvider) configuredProviderViolationsForType(ctx context.Context, c kclient.Client, providerType v1.ToolReferenceType) ([]ProviderViolation, error) {
	var refs v1.ToolReferenceList
	if err := c.List(ctx, &refs, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(providerType),
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list %s providers: %w", providerType, err)
	}

	var violations []ProviderViolation
	for _, ref := range refs.Items {
		if ref.Status.Tool == nil || !ref.Status.Configured {
			continue
		}
		meta, err := MetaForProvider(ref)
		if err != nil {
			return nil, err
		}

		missingEntitlements := p.Missing(meta.RequiredEntitlements)
		if len(missingEntitlements) == 0 {
			continue
		}

		violations = append(violations, ProviderViolation{
			Type:                 providerType,
			Namespace:            ref.Namespace,
			Name:                 ref.Name,
			RequiredEntitlements: meta.RequiredEntitlements,
			MissingEntitlements:  missingEntitlements,
		})
	}

	return violations, nil
}

// MetaForProvider extracts entitlement-related provider metadata from a tool reference.
func MetaForProvider(toolRef v1.ToolReference) (ProviderMeta, error) {
	var meta ProviderMeta
	if toolRef.Status.Tool == nil || toolRef.Status.Tool.Metadata["providerMeta"] == "" {
		return meta, nil
	}
	if err := json.Unmarshal([]byte(toolRef.Status.Tool.Metadata["providerMeta"]), &meta); err != nil {
		return meta, fmt.Errorf("failed to unmarshal provider meta for %s: %w", toolRef.Name, err)
	}
	return meta, nil
}

// fake is a fake handler that does fake things
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
