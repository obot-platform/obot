package authz

import (
	"context"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MetricsGroup         = "metrics"
	UnauthenticatedGroup = "unauthenticated"

	// anyGroup is an internal group that allows access to any group
	anyGroup = "*"
)

var (
	apiKeyOptionalSkillRoutes = newPathMatcher(
		"GET /api/skills",
		"GET /api/skills/{id}",
		"GET /api/skills/{id}/download",
	)
	adminAndOwnerRules = []string{
		"/api/tool-references",
		"/api/tool-references/",
		"/api/mcp-catalogs",
		"/api/mcp-catalogs/",
		"/api/mcp-servers",
		"/api/mcp-servers/",
		"/api/workspaces",
		"/api/workspaces/",
		"/api/mcp-webhook-validations",
		"/api/mcp-webhook-validations/",
		"/api/system-mcp-servers",
		"/api/system-mcp-servers/",
		"/api/system-mcp-catalogs",
		"/api/system-mcp-catalogs/",
		"GET /api/mcp-audit-logs",
		"GET /api/mcp-audit-logs/filter-options/{filter}",
		"GET /api/mcp-audit-logs/detail/{audit_log_id}",
		"GET /api/mcp-audit-logs/{mcp_id}",
		"GET /api/mcp-stats",
		"GET /api/mcp-stats/{mcp_id}",
		"GET /debug/pprof/",
		"GET /debug/triggers",
		"GET /debug/metrics",
		"/api/auth-providers",
		"/api/auth-providers/",
		"/api/model-providers",
		"/api/model-providers/",
		"GET /api/bookstrap",
		"/api/models",
		"/api/models/",
		"/api/model-access-policies",
		"/api/model-access-policies/",
		"/api/message-policies",
		"/api/message-policies/",
		"/api/message-policy-violations",
		"/api/message-policy-violations/",
		"GET /api/message-policy-violation-stats",
		"/api/devices/scans",
		"/api/devices/scans/",
		"/api/devices/scan-stats",
		"/api/devices/mcp-servers/",
		"/api/devices/skills",
		"/api/devices/skills/",
		"/api/devices/clients",
		"/api/devices/clients/",
		"/api/available-models",
		"/api/available-models/",
		"/api/default-model-aliases",
		"/api/default-model-aliases/",
		"GET /api/users",
		"GET /api/groups",
		"/api/group-role-assignments",
		"/api/group-role-assignments/",
		"POST /api/encrypt-all-users",
		"/api/users/",
		"GET /api/active-users",
		"GET /api/token-usage",
		"GET /api/total-token-usage",
		"GET /api/tokens",
		"DELETE /api/tokens/{id}",
		"/api/oauth-apps",
		"/api/oauth-apps/",
		"/api/user-default-role-settings",
		"/api/setup/",
		"/api/k8s-settings",
		"/api/image-pull-secrets",
		"/api/image-pull-secrets/",
		"/api/mcp-capacity",
		"/api/audit-log-exports",
		"/api/audit-log-exports/{id}",
		"/api/scheduled-audit-log-exports",
		"/api/scheduled-audit-log-exports/{id}",
		"/api/storage-credentials",
		"/api/storage-credentials/",
		"/api/oauth-clients",
		"/api/oauth-clients/",
		"/api/skill-repositories",
		"/api/skill-repositories/",
		"/api/skill-access-rules",
		"/api/skill-access-rules/",
		"GET /api/skills",
		"GET /api/skills/{id}",
		"GET /api/skills/{id}/download",
		"GET /api/eula",
		"PUT /api/eula",
		"PUT /api/app-preferences",

		// Allow admins to upload custom images
		"POST /api/image/upload",

		// This rule allows admins without an ACR to fetch tools for MCP servers in the default
		// catalog (all catalogs really) via the UI. It goes to the same handler as /api/mcp-servers/{mcpserver_id}/tools,
		// which admins already have access to from the rules above, so it's not exposing anything that
		// wasn't already accessible to them.
		// It's a bit of a hack, but it fixes the issue without refactoring the authz rules, changing the UI, or
		// adding local authz checks to the handler (like the rest of the /api/all-mcps/ endpoints).
		"GET /api/all-mcps/servers/{mcpserver_id}/tools",

		// Admin API key management endpoints
		"GET /api/admin-api-keys",
		"GET /api/admin-api-keys/{id}",
		"DELETE /api/admin-api-keys/{id}",

		"/api/projects",
		"/api/projects/",
		"GET /api/nanobot-agents",
	}
	staticRules = map[string][]string{
		types.GroupAdmin: adminAndOwnerRules,
		types.GroupOwner: adminAndOwnerRules,
		types.GroupAuditor: {
			"GET /api/admin-api-keys",
			"GET /api/admin-api-keys/{id}",
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/detail/{audit_log_id}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",
			"GET /api/mcp-capacity",
			"GET /api/users",
			"GET /api/users/",
			"GET /api/groups",
			"GET /api/groups/",
			"GET /api/group-role-assignments",
			"GET /api/group-role-assignments/",
			"GET /api/mcp-catalogs/",
			"GET /api/system-mcp-catalogs",
			"GET /api/system-mcp-catalogs/",
			"GET /api/mcp-webhook-validations",
			"GET /api/mcp-webhook-validations/",
			"GET /api/mcp-servers/",
			"GET /api/model-access-policies",
			"GET /api/model-access-policies/",
			"GET /api/message-policies",
			"GET /api/message-policies/",
			"GET /api/user-default-role-settings",
			"GET /api/k8s-settings",
			"POST /api/auth-providers/",
			"GET /api/workspaces/",
			"/api/audit-log-exports/",
			"/api/audit-log-exports/{id}",
			"/api/scheduled-audit-log-exports",
			"/api/scheduled-audit-log-exports/{id}",
			"/api/storage-credentials",
			"/api/storage-credentials/",
			"GET /api/skill-repositories",
			"GET /api/skill-repositories/",
			"GET /api/skill-access-rules",
			"GET /api/skill-access-rules/",
			"GET /api/skills",
			"GET /api/skills/{id}",
			"GET /api/skills/{id}/download",
			"GET /api/message-policy-violations",
			"GET /api/message-policy-violations/",
			"GET /api/message-policy-violations/filter-options/{filter}",
			"GET /api/message-policy-violations/{id}",
			"GET /api/message-policy-violation-stats",
			"GET /api/devices/scans",
			"GET /api/devices/scans/",
			"GET /api/devices/scan-stats",
			"GET /api/devices/mcp-servers/",
			"GET /api/devices/skills",
			"GET /api/devices/skills/",
			"GET /api/devices/clients",
			"GET /api/devices/clients/",
			"GET /api/token-usage",
			"GET /api/total-token-usage",
			"GET /api/nanobot-agents",
		},
		anyGroup: {
			// Allow access to the oauth2 endpoints
			"/oauth2/",

			"GET /api/token-request/{id}",
			"POST /api/token-request",
			"GET /api/token-request/{id}/{service}",

			"GET /api/oauth/start/{id}/{namespace}/{name}",

			"GET /api/bootstrap",
			"POST /api/bootstrap/login",
			"POST /api/bootstrap/logout",

			"GET /api/app-oauth/authorize/{id}",
			"GET /api/app-oauth/refresh/{id}",
			"GET /api/app-oauth/callback/{id}",
			"GET /api/app-oauth/get-token/{id}",
			"GET /api/app-oauth/get-token",

			"POST /api/sendgrid",

			"GET /api/healthz",

			"GET /api/app-preferences",

			"GET /api/auth-providers",
			"GET /api/auth-providers/{id}",

			"GET /api/tool-references",

			"GET /.well-known/",
			"POST /oauth/register/{mcp_id}",
			"POST /oauth/register",
			"GET /oauth/authorize/{mcp_id}",
			"GET /oauth/authorize",
			"POST /oauth/token/{mcp_id}",
			"POST /oauth/token",
			"GET /oauth/jwks.json",

			// Allow any user to read stored images.
			// This allows the UI to display custom images to unauthenticated users.
			"GET /api/image/{image_id}",

			// The auth for this is handled in the HTTP handler
			"POST /api/mcp-audit-logs",

			// API Key authentication webhook (called by nanobot shim)
			// This endpoint validates the API key passed in the header
			"POST /api/api-keys/auth",
		},

		types.GroupBasic: {
			"/api/llm-proxy/",
			"POST /api/prompt",
			"GET /api/models",
			"GET /api/model-providers",
			"GET /api/users",
			"GET /api/groups",

			// Allow authenticated users to read servers and entries from MCP catalogs.
			// The authz logic is handled in the routes themselves, for now.
			"GET /api/all-mcps/entries",
			"GET /api/all-mcps/entries/{entry_id}",
			"GET /api/all-mcps/servers",
			"GET /api/all-mcps/servers/{mcp_server_id}",

			// Audit log access for own servers (filtered in handler)
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/detail/{audit_log_id}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",

			// Published artifacts — any authenticated user can publish and search.
			// Artifact-specific access is enforced by resource authorization.
			"POST   /api/published-artifacts",
			"GET    /api/published-artifacts",

			// Skill discovery and download are filtered in the handler.
			"GET /api/skills",
			"GET /api/skills/{id}",
			"GET /api/skills/{id}/download",

			// Allow basic users to create and list ProjectV2 resources
			"POST /api/projects",
			"GET /api/projects",

			// Device scans: any authenticated user can submit a scan via
			// `obot scan`. Reads are admin/owner/auditor-only, gated by
			// the rules above.
			"POST /api/devices/scans",
		},

		types.GroupPowerUserPlus: {
			"GET /api/users",
			"GET /api/users/{user_id}",
			"GET /api/groups",
		},

		types.GroupPowerUser: {
			"GET /api/users",
			"GET /api/users/{user_id}",
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",
		},

		types.GroupAuthenticated: {
			"GET /oauth/userinfo",
			"GET /api/users",
			"GET /api/users/{user_id}",
			"GET /api/default-model-aliases",
			"/api/oauth/redirect/{namespace}/{name}",
			"GET /api/me",
			"DELETE /api/me",
			"PATCH /api/me",
			"POST /api/logout-all",
			"GET /api/version",
			"GET /api/setup/oauth-complete",

			// API key management for user's own keys
			"POST /api/api-keys",
			"GET /api/api-keys",
			"GET /api/api-keys/{id}",
			"DELETE /api/api-keys/{id}",
		},

		// API key users have restricted access - they can only access MCP-connect routes and /api/me
		// They get access to anyGroup routes automatically (health checks, OAuth flows, etc.)
		types.GroupAPIKey: {
			"GET /api/me",
			"GET /api/users",
			"GET /api/groups",
			"POST /api/published-artifacts",
			"GET /api/published-artifacts",
		},

		MetricsGroup: {
			"/debug/metrics",
		},
	}

	devModeRules = map[string][]string{
		anyGroup: {
			"/node_modules/",
			"/@fs/",
			"/.svelte-kit/",
			"/@vite/",
			"/@id/",
			"/src/",
		},
	}
)

type Authorizer struct {
	rules          []rule
	cache          kclient.Client
	uncached       kclient.Client
	apiResources   map[string]*pathMatcher
	uiResources    *pathMatcher
	acrHelper      *accesscontrolrule.Helper
	registryNoAuth bool
}

func NewAuthorizer(cache, uncached kclient.Client, devMode bool, acrHelper *accesscontrolrule.Helper, registryNoAuth bool) *Authorizer {
	apiBasedResources := make(map[string]*pathMatcher, len(apiResources))
	for group, resources := range apiResources {
		apiBasedResources[group] = newPathMatcher(resources...)
	}

	return &Authorizer{
		rules:          defaultRules(devMode, registryNoAuth),
		cache:          cache,
		uncached:       uncached,
		apiResources:   apiBasedResources,
		uiResources:    newPathMatcher(uiResources...),
		acrHelper:      acrHelper,
		registryNoAuth: registryNoAuth,
	}
}

func (a *Authorizer) Authorize(req *http.Request, user user.Info) bool {
	if authorizeAPIKeySkillRoutes(req, user) {
		return true
	}

	userGroups := user.GetGroups()
	for _, r := range a.rules {
		if r.group == anyGroup || slices.Contains(userGroups, r.group) {
			if _, pattern := r.mux.Handler(req); pattern != "" {
				return true
			}
		}
	}

	return a.authorizeAPIResources(req, user) || a.checkOAuthClient(req) || a.checkUI(req, user)
}

func authorizeAPIKeySkillRoutes(req *http.Request, user user.Info) bool {
	if !slices.Contains(user.GetGroups(), types.GroupAPIKey) {
		return false
	}

	if !slices.Contains(user.GetExtra()[types.APIKeySkillsAccessExtraKey], "true") {
		return false
	}

	_, ok := apiKeyOptionalSkillRoutes.Match(req)
	return ok
}

func (a *Authorizer) get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	err := a.cache.Get(ctx, key, obj, opts...)
	if apierrors.IsNotFound(err) {
		err = a.uncached.Get(ctx, key, obj, opts...)
	}
	return err
}

type rule struct {
	group string
	mux   *http.ServeMux
}

func defaultRules(devMode bool, registryNoAuth bool) []rule {
	var (
		rules []rule
		f     = (*fake)(nil)
	)

	for group := range staticRules {
		rule := rule{
			group: group,
			mux:   http.NewServeMux(),
		}
		for _, url := range staticRules[group] {
			rule.mux.Handle(url, f)
		}
		rules = append(rules, rule)
	}

	var registryRule rule
	if registryNoAuth {
		registryRule = rule{
			group: anyGroup,
			mux:   http.NewServeMux(),
		}
	} else {
		registryRule = rule{
			group: types.GroupBasic,
			mux:   http.NewServeMux(),
		}
	}
	registryRule.mux.Handle("GET /v0.1", f)
	registryRule.mux.Handle("GET /v0.1/", f)
	rules = append(rules, registryRule)

	if devMode {
		for group := range devModeRules {
			rule := rule{
				group: group,
				mux:   http.NewServeMux(),
			}
			for _, url := range devModeRules[group] {
				rule.mux.Handle(url, f)
			}
			rules = append(rules, rule)
		}
	}

	return rules
}

// fake is a fake handler that does fake things
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
