package license

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	enterpriseLimitEntitlementPrefix    = "OBOT_ENTERPRISE_"
	userLimitEntitlementUsersSuffix     = "_USERS"
	deviceLimitEntitlementDevicesSuffix = "_DEVICES"
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

	// features maps each license code that grants a feature to that feature.
	features = []struct {
		code    string
		feature func(*types.Entitlements) *bool
	}{
		{CloudEntitlement, func(e *types.Entitlements) *bool { return &e.Cloud }},
		{CommunityEntitlement, func(e *types.Entitlements) *bool { return &e.Community }},
		{EnterpriseEntitlement, func(e *types.Entitlements) *bool { return &e.Enterprise }},
		{EnterpriseAuthProvidersEntitlement, func(e *types.Entitlements) *bool { return &e.EnterpriseAuthProviders }},
		{EnterpriseModelProvidersEntitlement, func(e *types.Entitlements) *bool { return &e.EnterpriseModelProviders }},
	}
)

// Violation is a configured auth provider that needs a feature the entitlements lack, a limit usage is over, or a
// restriction.
type Violation struct {
	Type      string `json:"type"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ProviderEntitlementGate refuses gated paths while a configured auth provider needs a feature the entitlements lack.
type ProviderEntitlementGate struct {
	entitlements gatewayclient.EntitlementProvider
	client       kclient.Client
	mux          *http.ServeMux
}

func NewProviderEntitlementGate(entitlements gatewayclient.EntitlementProvider, client kclient.Client) *ProviderEntitlementGate {
	mux := http.NewServeMux()
	for _, path := range entitlementPathsToGate {
		mux.Handle(path, http.NotFoundHandler())
	}

	return &ProviderEntitlementGate{
		entitlements: entitlements,
		client:       client,
		mux:          mux,
	}
}

func (g *ProviderEntitlementGate) Check(req *http.Request) error {
	if g == nil || !g.requiresProviderEntitlements(req) {
		return nil
	}

	granted, err := g.entitlements.Entitlements(req.Context())
	if err != nil {
		return fmt.Errorf("failed to check provider license entitlements: %w", err)
	}
	violations, err := configuredAuthProviderViolations(req.Context(), g.client, granted)
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

// Entitlements returns the license's features and its user and device limits. A license limits nothing else.
func (p *Provider) Entitlements(ctx context.Context) (types.Entitlements, error) {
	if err := p.refresh(ctx, false); err != nil {
		return types.Entitlements{}, err
	}

	p.lock.RLock()
	defer p.lock.RUnlock()

	var entitlements types.Entitlements
	for _, feature := range features {
		if _, ok := p.entitlements[keygen.EntitlementCode(feature.code)]; ok {
			*feature.feature(&entitlements) = true
		}
	}
	entitlements.Users = p.resourceLimit(userLimitEntitlementUsersSuffix, gatewayclient.DefaultUserLimit)
	entitlements.Devices = p.resourceLimit(deviceLimitEntitlementDevicesSuffix, gatewayclient.DefaultDeviceLimit)
	return entitlements, nil
}

// resourceLimit adds up the license's OBOT_ENTERPRISE_<number><entitlementSuffix> entitlements. OBOT_ENTERPRISE
// without any is unlimited, and a license with neither gets defaultMaximum. The caller must hold p.lock.
func (p *Provider) resourceLimit(entitlementSuffix string, defaultMaximum int64) *int64 {
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

		entitlementMaximum, err := strconv.ParseInt(value, 10, 64)
		if err != nil || entitlementMaximum <= 0 {
			continue
		}

		if entitlementMaximum > math.MaxInt64-maximum {
			maximum = math.MaxInt64
		} else {
			maximum += entitlementMaximum
		}
	}

	if maximum == 0 {
		if isEnterpriseEdition {
			return nil
		}
		maximum = defaultMaximum
	}
	return &maximum
}

// EntitlementsMissing reports whether the environment's entitlements lack a feature that requiredEntitlements, a
// provider manifest's license codes, require.
func EntitlementsMissing(ctx context.Context, entitlements gatewayclient.EntitlementProvider, requiredEntitlements []string) (bool, error) {
	granted, err := entitlements.Entitlements(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to read entitlements: %w", err)
	}
	return missing(granted, requiredEntitlements), nil
}

// missing reports whether granted lacks a feature that codes require. A code for no feature Obot knows can't be
// granted, so it's missing too.
func missing(granted types.Entitlements, codes []string) bool {
	for _, code := range codes {
		var has bool
		for _, feature := range features {
			if feature.code == code {
				has = *feature.feature(&granted)
			}
		}
		if !has {
			return true
		}
	}
	return false
}

// Usage is what the environment uses against its limits, for display and the over-limit clock: the users and devices
// there are now, and the compute in the EntitlementUsage read from c. The leader of a billed environment keeps that
// count, so it can lag behind writes, and it's 0 where nothing counts it. A write is never admitted against it.
func Usage(ctx context.Context, gatewayClient *gatewayclient.Client, c kclient.Reader) (types.EntitlementsUsage, error) {
	var usage v1.EntitlementUsage
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.EntitlementUsageName}, &usage); kclient.IgnoreNotFound(err) != nil {
		return types.EntitlementsUsage{}, fmt.Errorf("failed to read the entitlement usage: %w", err)
	}
	used := types.EntitlementsUsage{
		MemoryMiB:     usage.Status.MemoryMiB,
		CPUMillicores: usage.Status.CPUMillicores,
	}

	var err error
	if used.Users, err = gatewayClient.UserCount(ctx); err != nil {
		return types.EntitlementsUsage{}, fmt.Errorf("failed to count users: %w", err)
	}
	if used.Devices, err = gatewayClient.DeviceCount(ctx); err != nil {
		return types.EntitlementsUsage{}, fmt.Errorf("failed to count devices: %w", err)
	}
	return used, nil
}

// LimitViolations returns a violation for each of granted's limits that used is over. A nil limit is unlimited. The
// messages leave out the numbers, since members see them too.
func LimitViolations(granted types.Entitlements, used types.EntitlementsUsage) []Violation {
	var violations []Violation
	for _, limit := range []struct {
		limit         *int64
		used          int64
		kind, message string
	}{
		{granted.Users, used.Users, "userLimit", "user count is over its limit"},
		{granted.Devices, used.Devices, "deviceLimit", "device count is over its limit"},
		{granted.MemoryMiB, used.MemoryMiB, "memoryLimit", "hosted MCP server memory is over its limit"},
		{granted.CPUMillicores, used.CPUMillicores, "cpuLimit", "hosted MCP server CPU is over its limit"},
	} {
		if limit.limit != nil && limit.used > *limit.limit {
			violations = append(violations, Violation{
				Type:    limit.kind,
				Message: limit.message,
			})
		}
	}
	return violations
}

// Violations returns the configured auth providers that need a feature granted lacks, and the limits used is over.
func Violations(ctx context.Context, c kclient.Client, granted types.Entitlements, used types.EntitlementsUsage) ([]Violation, error) {
	violations, err := configuredAuthProviderViolations(ctx, c, granted)
	if err != nil {
		return nil, err
	}
	return append(violations, LimitViolations(granted, used)...), nil
}

// configuredAuthProviderViolations returns the configured auth providers that need a feature granted lacks. Model
// providers aren't checked: the LLM gateway serves every model and requires no entitlement.
func configuredAuthProviderViolations(ctx context.Context, c kclient.Client, granted types.Entitlements) ([]Violation, error) {
	var authProviders v1.AuthProviderList
	if err := c.List(ctx, &authProviders, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list auth providers: %w", err)
	}

	var violations []Violation
	for _, ap := range authProviders.Items {
		if ap.Status.Configured && missing(granted, ap.Spec.RequiredEntitlements) {
			violations = append(violations, Violation{
				Type:      "authProvider",
				Namespace: ap.Namespace,
				Name:      ap.Name,
			})
		}
	}

	return violations, nil
}
