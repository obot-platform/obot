package authz

import (
	"net/http"
	"strings"
)

func (a *Authorizer) checkUI(req *http.Request, user User) bool {
	// The UI is registered as the "/" fallback on the main ServeMux. ServeMux
	// sets Request.Pattern before invoking the selected handler, so checking the
	// matched pattern prevents any explicitly registered backend route from
	// being authorized as UI traffic.
	if req.Pattern != "/" || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
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
		return user.IsAdmin || user.IsAuditor
	}

	// Did not hit any above conditions, so allow access.
	// Incorrect routes will be handled by the SvelteKit error page.
	return true
}
