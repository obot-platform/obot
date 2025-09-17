package authz

import (
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

var uiResources = []string{
	"GET /{$}",
	"GET /legacy-admin/",
	"GET /legacy-admin",
	"GET /admin/",
	"GET /admin",
	"GET /admin/assets/",
	"GET /agent/images/",
	"GET /landing/images/",
	"GET /_app/",
	"GET /{assistant}",
	"GET /chat",
	"GET /o/",
	"GET /s/",
	"GET /t/",
	"GET /i/{code}",
	"GET /user/images/",
	"GET /api/image/{id}",
	"GET /mcp-publisher",
	"GET /mcp-publisher/",
}

func (a *Authorizer) checkUI(req *http.Request, user user.Info) bool {
	vars, match := a.uiResources.Match(req)
	if !match {
		return false
	}
	if vars("assistant") == "api" {
		return false
	}

	// Allow all users to access /admin and /admin/
	if req.URL.Path == "/admin" || req.URL.Path == "/admin/" {
		return true
	}

	// Allow all users to access /admin/assets/
	if strings.HasPrefix(req.URL.Path, "/admin/assets/") {
		return true
	}

	// For /admin/ subroutes (but not /admin/ itself), only allow admin users
	if rest, ok := strings.CutPrefix(req.URL.Path, "/admin/"); ok && rest != "" {
		return slices.ContainsFunc(user.GetGroups(), func(group string) bool {
			return group == types.GroupAdmin || group == types.GroupOwner
		})
	}

	if strings.HasPrefix(req.URL.Path, "/mcp-publisher/") {
		return slices.Contains(user.GetGroups(), types.GroupPowerUser)
	}

	// Matches and is not API
	return true
}
