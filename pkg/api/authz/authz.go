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
	adminAndOwnerRules = []string{
		"/api/agents",
		"/api/agents/",
		"/api/projects",
		"/api/projects/",
		"/api/shares",
		"/api/shares/",
		"/api/tasks",
		"/api/tasks/",
		"/api/threads",
		"GET /api/threads/{id}",
		"GET /api/threads/{id}/files",
		"GET /api/threads/{id}/knowledge-files",
		"/api/tool-references",
		"/api/tool-references/",
		"GET /api/agents/{agent}/runs",
		"GET /api/agents/{agent}/threads/{thread}/runs",
		"GET /api/runs",
		"GET /api/runs/{id}",
		"GET /api/threads/{thread}/runs",
		"/api/webhooks",
		"/api/webhooks/",
		"/api/sendgrid",
		"/api/email-receivers",
		"/api/email-receivers/",
		"/api/cronjobs",
		"/api/cronjobs/",
		"/api/mcp-catalogs",
		"/api/mcp-catalogs/",
		"GET /api/mcp-catalogs/{catalog_id}/entries/all-servers",
		"/api/mcp-servers",
		"/api/mcp-servers/",
		"/api/workspaces",
		"/api/workspaces/",
		"/api/mcp-webhook-validations",
		"/api/mcp-webhook-validations/",
		"GET /api/mcp-audit-logs",
		"GET /api/mcp-audit-logs/filter-options/{filter}",
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
		"/api/file-scanner-providers",
		"/api/file-scanner-providers/",
		"GET /api/bookstrap",
		"/api/models",
		"/api/models/",
		"/api/available-models",
		"/api/available-models/",
		"/api/default-model-aliases",
		"/api/default-model-aliases/",
		"/api/workflows",
		"/api/workflows/",
		"GET /api/users",
		"GET /api/groups",
		"POST /api/encrypt-all-users",
		"/api/users/",
		"GET /api/active-users",
		"GET /api/token-usage",
		"GET /api/total-token-usage",
		"GET /api/tokens",
		"DELETE /api/tokens/{id}",
		"/api/oauth-apps",
		"/api/oauth-apps/",
		"/api/file-scanner-config",
		"/api/user-default-role-settings",
	}
	staticRules = map[string][]string{
		types.GroupAdmin: adminAndOwnerRules,
		types.GroupOwner: adminAndOwnerRules,
		types.GroupAuditor: {
			"GET /api/mcp-audit-logs",
			"GET /api/mcp-audit-logs/filter-options/{filter}",
			"GET /api/mcp-audit-logs/{mcp_id}",
			"GET /api/mcp-stats",
			"GET /api/mcp-stats/{mcp_id}",
			"GET /api/threads",
			"GET /api/threads/",
			"GET /api/runs",
			"GET /api/runs/",
			"GET /api/users",
			"GET /api/users/",
			"GET /api/groups",
			"GET /api/groups/",
			"GET /api/mcp-catalogs/",
			"GET /api/mcp-webhook-validations",
			"GET /api/mcp-webhook-validations/",
			"GET /api/mcp-servers/",
			"GET /api/tasks",
			"GET /api/tasks/",
			"GET /api/agents",
			"GET /api/default-model-aliases",
			"GET /api/user-default-role-settings",
			"POST /api/auth-providers/",
			"GET /api/workspaces/",
			"GET /api/projects/",
			"GET /api/assistants/{assistant_id}/projects/",
		},
		anyGroup: {
			// Allow access to the oauth2 endpoints
			"/oauth2/",

			"POST /api/webhooks/{namespace}/{id}",
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

			"GET /api/auth-providers",
			"GET /api/auth-providers/{id}",

			"POST /api/slack/events",

			// Allow public access to read display info for featured Obots
			// This is used in the unauthenticated landing page
			"GET /api/shares",
			"GET /api/templates",
			"GET /api/tool-references",

			"GET /.well-known/",
			"POST /oauth/register/{mcp_id}",
			"POST /oauth/register",
			"GET /oauth/authorize/{mcp_id}",
			"GET /oauth/authorize",
			"POST /oauth/token/{mcp_id}",
			"POST /oauth/token",
			"GET /oauth/callback/{oauth_request_id}",
			"GET /oauth/jwks.json",
		},

		types.GroupBasic: {
			"/api/assistants",
			"POST /api/llm-proxy/",
			"POST /api/prompt",
			"GET /api/models",
			"GET /api/model-providers",
			"POST /api/image/generate",
			"POST /api/image/upload",

			// Allow authenticated users to read and accept/reject project invitations.
			// The security depends on the code being an unguessable UUID string,
			// which is the project owner shares with the user that they are inviting.
			"GET /api/projectinvitations/{code}",
			"POST /api/projectinvitations/{code}",
			"DELETE /api/projectinvitations/{code}",

			// Allow authenticated users to read servers and entries from MCP catalogs.
			// The authz logic is handled in the routes themselves, for now.
			"GET /api/all-mcps/entries",
			"GET /api/all-mcps/entries/{entry_id}",
			"GET /api/all-mcps/servers",
			"GET /api/all-mcps/servers/{mcp_server_id}",
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
			"/api/oauth/redirect/{namespace}/{name}",
			"GET /api/me",
			"DELETE /api/me",
			"POST /api/logout-all",
			"GET /api/version",
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
	rules        []rule
	cache        kclient.Client
	uncached     kclient.Client
	apiResources map[string]*pathMatcher
	uiResources  *pathMatcher
	acrHelper    *accesscontrolrule.Helper
}

func NewAuthorizer(cache, uncached kclient.Client, devMode bool, acrHelper *accesscontrolrule.Helper) *Authorizer {
	apiBasedResources := make(map[string]*pathMatcher, len(apiResources))
	for group, resources := range apiResources {
		apiBasedResources[group] = newPathMatcher(resources...)
	}

	return &Authorizer{
		rules:        defaultRules(devMode),
		cache:        cache,
		uncached:     uncached,
		apiResources: apiBasedResources,
		uiResources:  newPathMatcher(uiResources...),
		acrHelper:    acrHelper,
	}
}

func (a *Authorizer) Authorize(req *http.Request, user user.Info) bool {
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

func defaultRules(devMode bool) []rule {
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
