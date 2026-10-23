// Package entitlements checks what the environment may do. Entitlements come from one Source: the Keygen license, or
// the billing subscription.
package entitlements

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RestrictedMessage is the error for requests refused while restricted.
	RestrictedMessage = "This environment is restricted until it's within its plan. An admin can reduce usage, and an owner can manage billing."
)

var (
	// GatedPaths are refused while a configured provider needs a missing entitlement, and for everyone while the
	// environment is restricted.
	GatedPaths = []string{
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

// Source provides the environment's entitlements. Services picks one at startup.
type Source interface {
	Entitlements(context.Context) (types.Entitlements, error)
	Usage(context.Context) (types.EntitlementsUsage, error)
	Restricted(context.Context) (bool, error)
}

// Violation is an auth provider missing a required entitlement, an exceeded limit, or a restriction.
type Violation struct {
	Type                 string   `json:"type"`
	Namespace            string   `json:"namespace"`
	Name                 string   `json:"name"`
	RequiredEntitlements []string `json:"requiredEntitlements"`
	MissingEntitlements  []string `json:"missingEntitlements"`
	Message              string   `json:"message"`
}

// Limits adapts a Source to the gateway's user and device limit interfaces.
type Limits struct {
	Source Source
}

// Codes returns the license codes the entitlements grant.
func Codes(entitlements types.Entitlements) []string {
	var codes []string
	for _, code := range []struct {
		code    string
		granted bool
	}{
		{license.CloudEntitlement, entitlements.Cloud},
		{license.CommunityEntitlement, entitlements.Community},
		{license.EnterpriseEntitlement, entitlements.Enterprise},
		{license.EnterpriseAuthProvidersEntitlement, entitlements.EnterpriseAuthProviders},
		{license.EnterpriseModelProvidersEntitlement, entitlements.EnterpriseModelProviders},
	} {
		if code.granted {
			codes = append(codes, code.code)
		}
	}
	return codes
}

// Missing returns the codes in required that the entitlements don't grant. Unknown codes are never granted.
func Missing(entitlements types.Entitlements, required []string) []string {
	granted := Codes(entitlements)
	var missing []string
	for _, code := range required {
		if !slices.Contains(granted, code) {
			missing = append(missing, code)
		}
	}
	return missing
}

// MissingEntitlements returns the codes in required that the source doesn't grant.
func MissingEntitlements(ctx context.Context, source Source, required []string) ([]string, error) {
	entitlements, err := source.Entitlements(ctx)
	if err != nil {
		return nil, err
	}
	return Missing(entitlements, required), nil
}

// RequireEntitlements returns Payment Required unless the source grants every code in required.
func RequireEntitlements(ctx context.Context, source Source, required []string) error {
	missing, err := MissingEntitlements(ctx, source, required)
	if err != nil {
		return fmt.Errorf("failed to read license entitlements: %w", err)
	}
	if len(missing) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("missing required license entitlements: %v", missing))
	}
	return nil
}

// LimitViolations returns a violation for each limit usage is over.
func LimitViolations(entitlements types.Entitlements, usage types.EntitlementsUsage) []Violation {
	var violations []Violation
	for _, limit := range []struct {
		violation, message string
		used               int64
		limit              *int64
	}{
		{"userLimit", "user count (%d) exceeds maximum limit (%d)", usage.Users, entitlements.Users},
		{"deviceLimit", "device count (%d) exceeds maximum limit (%d)", usage.Devices, entitlements.Devices},
		{"hostedMcpServerLimit", "running hosted MCP servers (%d) exceed maximum limit (%d)", usage.HostedMCPServers, entitlements.HostedMCPServers},
		{"memoryLimit", "hosted MCP server memory (%d MiB) exceeds maximum limit (%d MiB)", usage.MemoryMiB, entitlements.MemoryMiB},
		{"cpuLimit", "hosted MCP server CPU (%d millicores) exceeds maximum limit (%d millicores)", usage.CPUMillicores, entitlements.CPUMillicores},
	} {
		if limit.limit != nil && limit.used > *limit.limit {
			violations = append(violations, Violation{
				Type:    limit.violation,
				Message: fmt.Sprintf(limit.message, limit.used, *limit.limit),
			})
		}
	}
	return violations
}

// ProviderViolations returns configured auth providers that need entitlements the environment doesn't have. Model
// providers aren't checked, since the LLM gateway requires no entitlement.
func ProviderViolations(ctx context.Context, storage kclient.Client, entitlements types.Entitlements) ([]Violation, error) {
	var authProviders v1.AuthProviderList
	if err := storage.List(ctx, &authProviders, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, fmt.Errorf("failed to list auth providers: %w", err)
	}

	var violations []Violation
	for _, ap := range authProviders.Items {
		if missing := Missing(entitlements, ap.Spec.RequiredEntitlements); ap.Status.Configured && len(missing) > 0 {
			violations = append(violations, Violation{
				Type:                 "authProvider",
				Namespace:            ap.Namespace,
				Name:                 ap.Name,
				RequiredEntitlements: ap.Spec.RequiredEntitlements,
				MissingEntitlements:  missing,
			})
		}
	}
	return violations, nil
}

// Violations returns the provider, limit and restriction violations the version endpoint reports.
func Violations(ctx context.Context, source Source, storage kclient.Client) ([]Violation, error) {
	entitlements, err := source.Entitlements(ctx)
	if err != nil {
		return nil, err
	}
	violations, err := ProviderViolations(ctx, storage, entitlements)
	if err != nil {
		return nil, err
	}
	usage, err := source.Usage(ctx)
	if err != nil {
		return nil, err
	}
	violations = append(violations, LimitViolations(entitlements, usage)...)

	restricted, err := source.Restricted(ctx)
	if err != nil {
		return nil, err
	}
	if restricted {
		violations = append(violations, Violation{
			Type:    "restricted",
			Message: RestrictedMessage,
		})
	}
	return violations, nil
}

func (l Limits) UserLimit(ctx context.Context) (gatewayclient.UserLimit, error) {
	entitlements, err := l.Source.Entitlements(ctx)
	if err != nil {
		return gatewayclient.UserLimit{}, err
	}
	if entitlements.Users == nil {
		return gatewayclient.UserLimit{Unlimited: true}, nil
	}
	return gatewayclient.UserLimit{Maximum: *entitlements.Users}, nil
}

func (l Limits) DeviceLimit(ctx context.Context) (gatewayclient.DeviceLimit, error) {
	entitlements, err := l.Source.Entitlements(ctx)
	if err != nil {
		return gatewayclient.DeviceLimit{}, err
	}
	if entitlements.Devices == nil {
		return gatewayclient.DeviceLimit{Unlimited: true}, nil
	}
	return gatewayclient.DeviceLimit{Maximum: *entitlements.Devices}, nil
}
