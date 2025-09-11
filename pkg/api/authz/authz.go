package authz

import (
	"context"
	"maps"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AdminGroup           = "admin"
	PowerUserPlusGroup   = "power-user-plus"
	PowerUserGroup       = "power-user"
	AuthenticatedGroup   = "authenticated"
	MetricsGroup         = "metrics"
	UnauthenticatedGroup = "unauthenticated"

	// anyGroup is an internal group that allows access to any group
	anyGroup = "*"
)

var staticRules = map[string][]string{
	AdminGroup: {
		// Yay! Everything
		"/",
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

	AuthenticatedGroup: {
		"/api/oauth/redirect/{namespace}/{name}",
		"/api/assistants",
		"GET /api/me",
		"DELETE /api/me",
		"POST /api/llm-proxy/",
		"POST /api/prompt",
		"GET /api/models",
		"GET /api/model-providers",
		"GET /api/version",
		"POST /api/image/generate",
		"POST /api/image/upload",
		"POST /api/logout-all",

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

	PowerUserPlusGroup: {
		// Access Control Rules
		"GET /api/workspaces/{workspace_id}/access-control-rules",
		"GET /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
		"POST /api/workspaces/{workspace_id}/access-control-rules",
		"PUT /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",
		"DELETE /api/workspaces/{workspace_id}/access-control-rules/{access_control_rule_id}",

		// Workspace-scoped MCP Server Catalog Entries
		"GET /api/workspaces/{workspace_id}/entries",
		"GET /api/workspaces/{workspace_id}/entries/{entry_id}",
		"POST /api/workspaces/{workspace_id}/entries",
		"PUT /api/workspaces/{workspace_id}/entries/{entry_id}",
		"DELETE /api/workspaces/{workspace_id}/entries/{entry_id}",
		"POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews",
		"POST /api/workspaces/{workspace_id}/entries/{entry_id}/generate-tool-previews/oauth-url",

		// Workspace-scoped MCP Servers
		"GET /api/workspaces/{workspace_id}/servers",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
		"POST /api/workspaces/{workspace_id}/servers",
		"PUT /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
		"DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}",
		"POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/launch",
		"POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/check-oauth",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth-url",
		"DELETE /api/workspaces/{workspace_id}/servers/{mcp_server_id}/oauth",
		"POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/configure",
		"POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/deconfigure",
		"POST /api/workspaces/{workspace_id}/servers/{mcp_server_id}/reveal",
		"GET /api/workspaces/{workspace_id}/servers/{mcp_server_id}/instances",

		"GET /api/users",
		"GET /api/users/{user_id}",
		"GET /api/groups",
	},

	MetricsGroup: {
		"/debug/metrics",
	},
}

var devModeRules = map[string][]string{
	anyGroup: {
		"/node_modules/",
		"/@fs/",
		"/.svelte-kit/",
		"/@vite/",
		"/@id/",
		"/src/",
	},
}

type Authorizer struct {
	rules        []rule
	cache        kclient.Client
	uncached     kclient.Client
	apiResources *pathMatcher
	uiResources  *pathMatcher
	acrHelper    *accesscontrolrule.Helper
}

func NewAuthorizer(cache, uncached kclient.Client, devMode bool, acrHelper *accesscontrolrule.Helper) *Authorizer {
	return &Authorizer{
		rules:        defaultRules(devMode),
		cache:        cache,
		uncached:     uncached,
		apiResources: newPathMatcher(apiResources...),
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

	for _, group := range slices.Sorted(maps.Keys(staticRules)) {
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
		for _, group := range slices.Sorted(maps.Keys(devModeRules)) {
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
