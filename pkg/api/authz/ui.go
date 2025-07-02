package authz

import (
	"net/http"
	"slices"
	"strings"

	"k8s.io/apiserver/pkg/authentication/user"
)

var uiResources = []string{
	"GET /{$}",
	"GET /admin/",
	"GET /v2/admin",
	"GET /v2/admin/",
	"GET /agent/images/",
	"GET /landing/images/",
	"GET /_app/",
	"GET /{assistant}",
	"GET /o/",
	"GET /s/",
	"GET /t/",
	"GET /i/{code}",
	"GET /user/images/",
	"GET /api/image/{id}",
}

func (a *Authorizer) checkUI(req *http.Request, user user.Info) bool {
	vars, match := a.uiResources.Match(req)
	if !match {
		return false
	}
	if vars("assistant") == "api" {
		return false
	}

	// Allow all users to access /v2/admin and /v2/admin/
	if req.URL.Path == "/v2/admin" || req.URL.Path == "/v2/admin/" {
		return true
	}

	// For /v2/admin/ subroutes (but not /v2/admin/ itself), only allow admin users
	if strings.HasPrefix(req.URL.Path, "/v2/admin/") && req.URL.Path != "/v2/admin/" {
		return slices.Contains(user.GetGroups(), AdminGroup)
	}

	// Matches and is not API
	return true
}
