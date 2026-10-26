package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/entitlements"
	"github.com/obot-platform/obot/pkg/mcp/rpcerror"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/proxy"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// restrictedLoginPath is the login page that a restricted user's page load is sent to, with restrictedCookie. Its
	// parameter tells a refused load of the login page itself apart, which isn't sent there again.
	restrictedLoginPath = "/?restricted=true"
	// restrictedCookie tells the login page that its visitor was signed out because the environment is restricted. The
	// UI reads and clears it (ENVIRONMENT_RESTRICTED_COOKIE in ui/user/src/lib/constants.ts).
	restrictedCookie = "obot_environment_restricted"
)

var (
	// errRestricted refuses a request from a user who can't use the environment while it's restricted.
	errRestricted = types.NewErrHTTP(http.StatusPaymentRequired, entitlements.RestrictedMessage)
	// deviceRequests are what managed devices send. They're allowed while restricted, so devices keep enforcing policy.
	deviceRequests = []string{
		"POST /api/local-agent-audit-logs",
		"POST /api/enforcement/decisions",
		"POST /api/mdm/enroll",
		"POST /api/devices/scans",
	}
)

func gatedPaths() *http.ServeMux {
	mux := http.NewServeMux()
	for _, path := range entitlements.GatedPaths {
		mux.Handle(path, http.NotFoundHandler())
	}
	return mux
}

// entitlementGate refuses:
//   - requests from users who aren't admins or owners while the plan restricts the environment, so admins fix it. The
//     refusal is errRestricted, which signs them out;
//   - gated paths while a configured auth provider needs a missing entitlement, as without billing.
func (s *Server) entitlementGate(req *http.Request, user kuser.Info) error {
	if s.entitlements == nil {
		return nil
	}
	_, pattern := s.gatedPaths.Handler(req)
	gated := pattern != ""
	restrictable := s.restrictions && s.restrictable(req, user)
	if !gated && !restrictable {
		return nil
	}

	// With billing, the plan is the entitlements, so one read answers both checks.
	granted, err := s.entitlements.Entitlements(req.Context())
	if err != nil {
		return fmt.Errorf("failed to check license entitlements: %w", err)
	}
	if restrictable && granted.Restricted {
		return errRestricted
	}
	if !gated {
		return nil
	}
	violations, err := entitlements.ProviderViolations(req.Context(), s.storageClient, granted)
	if err != nil {
		return fmt.Errorf("failed to check provider license entitlements: %w", err)
	}
	if len(violations) > 0 {
		return types.NewErrHTTP(http.StatusPaymentRequired, "configured provider is missing required license entitlements")
	}
	return nil
}

// restrictable returns whether a restriction applies to the request: it's from a signed-in user who isn't an admin or
// owner, and it isn't for a static asset, sign out, or a device request.
func (s *Server) restrictable(req *http.Request, user kuser.Info) bool {
	return slices.Contains(user.GetGroups(), types.GroupAuthenticated) &&
		!principal.IsAdminOrOwner(user) &&
		!isStaticAssetPath(req.URL.Path) &&
		req.URL.Path != "/oauth2/sign_out" &&
		!slices.Contains(deviceRequests, req.Method+" "+req.URL.Path)
}

// answerRefusedMCPCall answers an MCP client's call that the gate refused with a JSON-RPC error, so the client shows
// its user why. It reports whether it answered. Errors from the MCP gateway itself keep their HTTP status, which
// clients act on, like a 404 for an expired session.
func answerRefusedMCPCall(rw http.ResponseWriter, req *http.Request, refusal error) bool {
	httpErr, ok := errors.AsType[*types.ErrHTTP](refusal)
	if !ok || !strings.HasPrefix(req.URL.Path, "/mcp-connect/") {
		return false
	}
	response, ok := rpcerror.Answer(req, httpErr.Message)
	if !ok {
		return false
	}
	rw.Header().Set("Content-Type", "application/json")
	_, _ = rw.Write(response)
	return true
}

// signOutRestricted ends the browser session of a user refused by the restriction, and tells the login page why. It
// sends a page load to the login page and reports that it answered. Anything else is refused as usual, and the UI
// follows the cookie to the login page.
func signOutRestricted(rw http.ResponseWriter, req *http.Request) bool {
	http.SetCookie(rw, &http.Cookie{
		Name:   proxy.ObotAccessTokenCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.SetCookie(rw, &http.Cookie{
		Name:     restrictedCookie,
		Value:    "true",
		Path:     "/",
		MaxAge:   60,
		SameSite: http.SameSiteLaxMode,
	})
	if !isPageLoad(req) || req.URL.String() == restrictedLoginPath {
		return false
	}
	http.Redirect(rw, req, restrictedLoginPath, http.StatusFound)
	return true
}
