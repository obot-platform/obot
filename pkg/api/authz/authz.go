package authz

import (
	"net/http"
	"slices"

	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	AdminGroup           = "admin"
	AuthenticatedGroup   = "authenticated"
	UnauthenticatedGroup = "unauthenticated"

	// anyGroup is an internal group that allows access to any group
	anyGroup = "*"
)

type Authorizer struct {
	rules []rule
}

func NewAuthorizer() *Authorizer {
	return &Authorizer{
		rules: defaultRules(),
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

	return false
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

	// Build admin mux, admins can assess any URL
	adminMux := http.NewServeMux()
	adminMux.Handle("/", f)

	rules = append(rules, rule{
		group: AdminGroup,
		mux:   adminMux,
	})

	// Build mux that anyone can access
	anyMux := http.NewServeMux()
	anyMux.Handle("POST /api/webhooks/{id}", f)

	anyMux.Handle("GET /api/token-request/{id}", f)
	anyMux.Handle("POST /api/token-request", f)
	anyMux.Handle("GET /api/token-request/{id}/{service}", f)

	anyMux.Handle("GET /api/auth-providers", f)
	anyMux.Handle("GET /api/auth-providers/{slug}", f)

	anyMux.Handle("GET /api/oauth/start/{id}/{service}", f)
	anyMux.Handle("/api/oauth/redirect/{service}", f)

	anyMux.Handle("GET /api/app-oauth/authorize/{id}", f)
	anyMux.Handle("GET /api/app-oauth/refresh/{id}", f)
	anyMux.Handle("GET /api/app-oauth/callback/{id}", f)
	anyMux.Handle("GET /api/app-oauth/get-token", f)

	rules = append(rules, rule{
		group: anyGroup,
		mux:   anyMux,
	})

	return rules
}

// fake is a fake handler that does nothing
type fake struct{}

func (f *fake) ServeHTTP(http.ResponseWriter, *http.Request) {}
