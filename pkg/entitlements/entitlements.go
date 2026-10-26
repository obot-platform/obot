// Package entitlements checks what the environment may do. Entitlements come from the Keygen license, or from the
// billing plan when billing is on.
package entitlements

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// RestrictedMessage is the error for requests refused while the environment is restricted.
	RestrictedMessage = "This environment is over its plan, so only admins can use it right now. An admin can bring usage back within the plan, or an owner can change it."
)

var (
	// GatedPaths are refused while a configured auth provider needs a missing entitlement.
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

	// Users, Devices, Memory and CPU are the limits a plan can set.
	Users = Limit{
		violation: "userLimit",
		name:      "user count",
		limit:     func(e types.Entitlements) *int64 { return e.Users },
		used:      func(u types.EntitlementsUsage) int64 { return u.Users },
	}
	Devices = Limit{
		violation: "deviceLimit",
		name:      "device count",
		limit:     func(e types.Entitlements) *int64 { return e.Devices },
		used:      func(u types.EntitlementsUsage) int64 { return u.Devices },
	}
	Memory = Limit{
		violation: "memoryLimit",
		name:      "hosted MCP server memory",
		limit:     func(e types.Entitlements) *int64 { return e.MemoryMiB },
		used:      func(u types.EntitlementsUsage) int64 { return u.MemoryMiB },
	}
	CPU = Limit{
		violation: "cpuLimit",
		name:      "hosted MCP server CPU",
		limit:     func(e types.Entitlements) *int64 { return e.CPUMillicores },
		used:      func(u types.EntitlementsUsage) int64 { return u.CPUMillicores },
	}

	// allLimits are every limit, in the order their violations are reported.
	allLimits = []Limit{Users, Devices, Memory, CPU}
)

// Reader reads the environment's entitlements: the Keygen license's without billing, or the billing plan's.
type Reader interface {
	Entitlements(context.Context) (types.Entitlements, error)
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

// Limit is one of the plan's limits: the violation it reports, what messages call it, and how to read it from the
// entitlements and the usage.
type Limit struct {
	violation string
	name      string
	limit     func(types.Entitlements) *int64
	used      func(types.EntitlementsUsage) int64
}

// Missing returns the codes in required that the entitlements don't grant. A code for no known feature is never
// granted.
func Missing(entitlements types.Entitlements, required []string) []string {
	granted := license.Codes(entitlements)
	var missing []string
	for _, code := range required {
		if !slices.Contains(granted, code) {
			missing = append(missing, code)
		}
	}
	return missing
}

// MissingEntitlements returns the codes in required that the environment doesn't have.
func MissingEntitlements(ctx context.Context, reader Reader, required []string) ([]string, error) {
	entitlements, err := reader.Entitlements(ctx)
	if err != nil {
		return nil, err
	}
	return Missing(entitlements, required), nil
}

// RequireEntitlements returns Payment Required unless the environment has every code in required.
func RequireEntitlements(ctx context.Context, reader Reader, required []string) error {
	missing, err := MissingEntitlements(ctx, reader, required)
	if err != nil {
		return fmt.Errorf("failed to refresh license entitlements: %w", err)
	}
	if len(missing) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("missing required license entitlements: %v", missing))
	}
	return nil
}

// ProviderViolations returns the configured auth providers that require codes the entitlements don't grant. Model
// providers aren't checked, since the LLM gateway requires no entitlement.
func ProviderViolations(ctx context.Context, storage kclient.Client, entitlements types.Entitlements) ([]Violation, error) {
	var authProviders v1.AuthProviderList
	if err := storage.List(ctx, &authProviders, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, fmt.Errorf("failed to list auth providers: %w", err)
	}

	var violations []Violation
	for _, ap := range authProviders.Items {
		if !ap.Status.Configured {
			continue
		}
		if missing := Missing(entitlements, ap.Spec.RequiredEntitlements); len(missing) > 0 {
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

// Exceeded returns Payment Required if going from before to after raises one of limits past the plan's. A limit the
// change doesn't raise isn't checked, so changes that lower usage always pass, even while over. The message names the
// limit but not the numbers, since members see it too.
func Exceeded(limits []Limit, plan types.Entitlements, before, after types.EntitlementsUsage) error {
	for _, l := range limits {
		limit, used := l.limit(plan), l.used(after)
		if limit != nil && used > l.used(before) && used > *limit {
			return types.NewErrHTTP(http.StatusPaymentRequired, fmt.Sprintf("There isn't room on the plan's %s for this. Free some up, or ask an owner to change the plan.", l.name))
		}
	}
	return nil
}

// LimitViolations returns a violation for each limit usage is over. Like Exceeded's, the messages leave out the numbers,
// since members see them too.
func LimitViolations(plan types.Entitlements, usage types.EntitlementsUsage) []Violation {
	var violations []Violation
	for _, l := range allLimits {
		if limit, used := l.limit(plan), l.used(usage); limit != nil && used > *limit {
			violations = append(violations, Violation{
				Type:    l.violation,
				Message: fmt.Sprintf("%s is over its limit", l.name),
			})
		}
	}
	return violations
}
