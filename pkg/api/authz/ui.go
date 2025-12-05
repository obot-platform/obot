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
	// Reject direct access to /api or /api paths for UI except for /api/image/{id}
	if req.URL.Path == "/api" || (strings.HasPrefix(req.URL.Path, "/api/") && !strings.HasPrefix(req.URL.Path, "/api/image/")) {
		return false
	}

	// Allow all users to access /admin/assets/
	if strings.HasPrefix(req.URL.Path, "/admin/assets/") {
		return true
	}

	// Allow all users to access /admin and /admin/
	if req.URL.Path == "/admin" || req.URL.Path == "/admin/" {
		return true
	}

	// For /admin/ subroutes, if user has auditor or admin group
	if rest, ok := strings.CutPrefix(req.URL.Path, "/admin/"); ok && rest != "" {
		return slices.ContainsFunc(user.GetGroups(), func(group string) bool {
			return group == types.GroupAdmin || group == types.GroupOwner || group == types.GroupAuditor
		})
	}

	if strings.HasPrefix(req.URL.Path, "/mcp-publisher/") {
		return slices.Contains(user.GetGroups(), types.GroupPowerUser)
	}

	// did not hit any above conditions, so allow access
	// incorrect routes will handled by SvelteKit error page
	return true
}
