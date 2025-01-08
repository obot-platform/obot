package authz

import (
	"maps"
	"net/http"
	"slices"

	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AdminGroup           = "admin"
	AuthenticatedGroup   = "authenticated"
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
		// Allow access to the UI
		"/admin/",
		"/{$}",
		"/static/",
		// Allow access to the oauth2 endpoints
		"/oauth2/",

		"POST /api/webhooks/{namespace}/{id}",
		"GET /api/token-request/{id}",
		"POST /api/token-request",
		"GET /api/token-request/{id}/{service}",

		"GET /api/auth-providers",
		"GET /api/auth-providers/{slug}",

		"GET /api/oauth/start/{id}/{service}",

		"GET /api/app-oauth/authorize/{id}",
		"GET /api/app-oauth/refresh/{id}",
		"GET /api/app-oauth/callback/{id}",
		"GET /api/app-oauth/get-token",
	},
	AuthenticatedGroup: {
		"/api/oauth/redirect/{service}",
		"/api/assistants/{path...}",
		"GET /api/me",
		"PATCH /api/users/{id}",
		"POST /api/llm-proxy/",
		"POST /api/prompt",
		"GET /api/models",
		"GET /api/version",
	},
}

type Authorizer struct {
	rules   []rule
	storage kclient.Client
}

func NewAuthorizer(storage kclient.Client) *Authorizer {
	return &Authorizer{
		rules:   defaultRules(),
		storage: storage,
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

	if authorizeThread(req, user) {
		return true
	}

	return a.authorizeThreadFileDownload(req, user)
}

type rule struct {
	group string
	mux   *http.ServeMux
}

func defaultRules() []rule {
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

	return rules
}

// fake is a fake handler that does fake things
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
