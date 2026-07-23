package authz

import (
	"context"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
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
	adminAndOwnerRules = []string{
		"/api/mcp-catalogs",
		"/api/mcp-catalogs/",
		"/api/model-info-source",
		"/api/model-info-source/",
		"GET /api/mcp-server-binding-secrets",
		"/api/mcp-servers",
		"/api/mcp-servers/",
		"/api/all-mcps",
		"/api/all-mcps/",
		"/api/workspaces",
		"/api/workspaces/",
		"/api/mcp-webhook-validations",
		"/api/mcp-webhook-validations/",
		"/api/system-mcp-servers",
		"/api/system-mcp-servers/",
		"/api/system-mcp-catalogs",
		"/api/system-mcp-catalogs/",
		"GET /api/mcp-audit-logs",
		"GET /api/mcp-audit-logs/",
		"GET /api/llm-audit-logs",
		"GET /api/llm-audit-logs/",
		"GET /api/llm-audit-logs/filter-options/",
		"GET /api/mcp-stats",
		"GET /api/mcp-stats/",
		"GET /debug/pprof/",
		"GET /debug/triggers",
		"GET /debug/metrics",
		"PUT /api/license",
		"POST /api/license",
		"DELETE /api/license",
		"/api/auth-providers",
		"/api/auth-providers/",
		"/api/local-auth/users",
		"/api/local-auth/users/",
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
		"/api/devices/scan-stats",
		"/api/devices/mcp-servers/",
		"/api/devices/skills",
		"/api/devices/skills/",
		"/api/devices/clients",
		"/api/devices/clients/",
		"/api/mdm/configurations",
		"/api/mdm/configurations/",
		"GET /api/enforcement-decisions",
		"GET /api/enforcement-decisions/",
		"/api/mdm/asset-source",
		"/api/mdm/asset-source/",
		"GET /api/mdm/assets",
		"/api/available-models",
		"/api/available-models/",
		"/api/default-model-aliases",
		"/api/default-model-aliases/",
		"/api/users",
		"GET /api/groups",
		"/api/group-role-assignments",
		"/api/group-role-assignments/",
		"POST /api/encrypt-all-users",
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
		"/api/git-credentials",
		"/api/git-credentials/",
		"/api/mcp-capacity",
		"/api/audit-log-exports",
		"/api/audit-log-exports/",
		"/api/scheduled-audit-log-exports",
		"/api/scheduled-audit-log-exports/",
		"/api/storage-credentials",
		"/api/storage-credentials/",
		"/api/oauth-clients",
		"/api/oauth-clients/",
		"/api/skill-repositories",
		"/api/skill-repositories/",
		"/api/skill-access-rules",
		"/api/skill-access-rules/",
		"GET /api/eula",
		"PUT /api/eula",
		"PUT /api/app-preferences",
		"PUT /api/app-notification",

		// Allow admins to upload custom images
		"POST /api/image/upload",

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
			"GET /api/mcp-audit-logs/",
			"GET /api/llm-audit-logs",
			"GET /api/llm-audit-logs/",
			"GET /api/llm-audit-logs/filter-options/",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/",
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
			"GET /api/image-pull-secrets/capability",
			"GET /api/image-pull-secrets",
			"GET /api/image-pull-secrets/",
			"GET /api/git-credentials",
			"GET /api/git-credentials/",
			"POST /api/auth-providers/",
			"GET /api/local-auth/users",
			"GET /api/local-auth/users/",
			"GET /api/workspaces/",
			"/api/audit-log-exports",
			"/api/audit-log-exports/",
			"/api/scheduled-audit-log-exports",
			"/api/scheduled-audit-log-exports/",
			"/api/storage-credentials",
			"/api/storage-credentials/",
			"GET /api/skill-repositories",
			"GET /api/skill-repositories/",
			"GET /api/skill-access-rules",
			"GET /api/skill-access-rules/",
			"GET /api/message-policy-violations",
			"GET /api/message-policy-violations/",
			"GET /api/message-policy-violation-stats",
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

			"GET /api/healthz",

			"GET /api/app-preferences",

			"GET /api/auth-providers",
			"GET /api/auth-providers/{id}",

			"GET  /.well-known/",
			"POST /oauth/register/{mcp_id}",
			"POST /oauth/register",
			"GET  /oauth/authorize/{mcp_id}",
			"GET  /oauth/authorize",
			"POST /oauth/token/{mcp_id}",
			"POST /oauth/token",
			"GET  /oauth/jwks.json",
			"GET  /oauth/client-metadata.json",

			// Allow any user to read stored images.
			// This allows the UI to display custom images to unauthenticated users.
			"GET /api/image/{image_id}",

			// The auth for this is handled in the HTTP handler
			"POST /api/mcp-audit-logs",

			// API Key authentication webhook (called by nanobot shim)
			// This endpoint validates the API key passed in the header
			"POST /api/api-keys/auth",
		},

		types.GroupAPI: {
			"POST /api/local-agent-audit-logs",
			"GET /api/models",
			"GET /api/model-providers",
			"GET /api/users",
			"GET /api/groups",

			"GET /api/app-notification",

			// Allow authenticated users to read servers and entries from MCP catalogs.
			// Filtering is handled in the handler.
			"GET /api/all-mcps/entries",
			"GET /api/all-mcps/servers",

			// Audit log access for own servers (filtered in handler)
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/detail/{audit_log_id}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",

			// Allow basic users to create and list projects
			"POST /api/projects",
			"GET /api/projects",

			// API key management for user's own keys
			"POST /api/api-keys",
			"GET /api/api-keys",
			"GET /api/api-keys/{id}",
			"DELETE /api/api-keys/{id}",

			"GET /api/users",
			"GET /api/users/{user_id}",
			"GET /api/default-model-aliases",
			"/api/oauth/redirect/{namespace}/{name}",
			"DELETE /api/me",
			"PATCH /api/me",
			"POST /api/logout-all",
			"GET /api/version",
			"GET /api/default-k8s-settings",
			"GET /api/license",
			"GET /api/setup/oauth-complete",
		},

		types.GroupPowerUserPlus: {
			"GET /api/groups",
		},

		types.GroupPowerUser: {
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",
		},

		types.GroupAuthenticated: {
			"GET /oauth/userinfo",
			"GET /api/me",
		},

		types.GroupSkills: {
			// Skill discovery and download are filtered in the handler.
			"GET /api/skills",
		},

		types.GroupPublishedArtifacts: {
			// Published artifacts — any authenticated user can publish and search.
			// Artifact-specific access is enforced by resource authorization.
			"POST   /api/published-artifacts",
			"GET    /api/published-artifacts",
		},

		types.GroupLLM: {
			"/api/llm-proxy/",
		},

		types.GroupDeviceScans: {
			// Device scans: any authenticated user can submit a scan via
			// `obot scan` and read the scans they themselves submitted.
			// Clamp list results to SubmittedBy == req.User.GetUID()
			"POST /api/devices/scans",
			"GET /api/devices/scans",

			// Credentials that can submit scans can also submit local agent tool call audit logs.
			"POST /api/local-agent-audit-logs",

			// Devices ask for a synchronous enforcement decision before running a tool call.
			"POST /api/enforcement/decisions",
		},

		types.GroupDeviceEnroll: {
			// A device enrollment token authenticates as its configuration and may
			// only enroll a device — nothing else.
			"POST /api/mdm/enroll",
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
	gatewayClient  *client.Client
	apiResources   map[string]*pathMatcher
	uiResources    *pathMatcher
	acrHelper      *accesscontrolrule.Helper
	skillHelper    *skillaccessrule.Helper
	registryNoAuth bool
}

func NewAuthorizer(gatewayClient *client.Client, cache, uncached kclient.Client, devMode bool, acrHelper *accesscontrolrule.Helper, skillHelper *skillaccessrule.Helper, registryNoAuth bool) *Authorizer {
	apiBasedResources := make(map[string]*pathMatcher, len(apiResources))
	for group, resources := range apiResources {
		apiBasedResources[group] = newPathMatcher(resources...)
	}

	return &Authorizer{
		rules:          defaultRules(devMode, registryNoAuth),
		cache:          cache,
		uncached:       uncached,
		gatewayClient:  gatewayClient,
		apiResources:   apiBasedResources,
		uiResources:    newPathMatcher(uiResources...),
		acrHelper:      acrHelper,
		skillHelper:    skillHelper,
		registryNoAuth: registryNoAuth,
	}
}

func (a *Authorizer) Authorize(req *http.Request, userInfo user.Info) bool {
	user := newUser(userInfo)
	for _, r := range a.rules {
		if r.group == anyGroup || slices.Contains(user.GetGroups(), r.group) {
			if _, pattern := r.mux.Handler(req); pattern != "" {
				return true
			}
		}
	}

	return a.authorizeAPIResources(req, user) || a.checkOAuthClient(req) || a.checkUI(req, user)
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
		f     = (*fake)(nil)
		rules = rulesFromStatic(staticRules)
	)

	var registryRuleBasic, registryRuleMCPOAuth rule
	if registryNoAuth {
		registryRuleBasic = rule{
			group: anyGroup,
			mux:   http.NewServeMux(),
		}
		registryRuleMCPOAuth = rule{
			group: anyGroup,
			mux:   http.NewServeMux(),
		}
	} else {
		registryRuleBasic = rule{
			group: types.GroupBasic,
			mux:   http.NewServeMux(),
		}
		registryRuleMCPOAuth = rule{
			group: types.GroupMCP,
			mux:   http.NewServeMux(),
		}
	}
	registryRuleBasic.mux.Handle("GET /v0.1", f)
	registryRuleBasic.mux.Handle("GET /v0.1/", f)
	registryRuleMCPOAuth.mux.Handle("GET /v0.1", f)
	registryRuleMCPOAuth.mux.Handle("GET /v0.1/", f)
	rules = append(rules, registryRuleBasic, registryRuleMCPOAuth)

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

func rulesFromStatic(static map[string][]string) []rule {
	var (
		f     = (*fake)(nil)
		rules []rule
	)
	for group := range static {
		rule := rule{
			group: group,
			mux:   http.NewServeMux(),
		}
		seen := map[string]struct{}{}
		for _, url := range static[group] {
			if _, ok := seen[url]; ok {
				continue
			}
			seen[url] = struct{}{}
			rule.mux.Handle(url, f)
		}
		rules = append(rules, rule)
	}
	return rules
}

// fake is a fake handler that does fake things
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
