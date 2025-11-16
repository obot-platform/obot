package setup

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/bootstrap"
)

// OAuthComplete handles the OAuth callback for setup flow.
// This endpoint is called after oauth2-proxy completes authentication.
// Any authenticated user can be cached - they don't need to be pre-configured
// as an Owner. The bootstrap user will review their details and decide whether
// to confirm them as the first Owner.
// Endpoint: GET /api/setup/oauth-complete
func (h *Handler) OAuthComplete(req api.Context) error {
	// If the user that just logged in is an Owner, then we can redirect them now.
	// The setup routes will be disabled, so we can just send the owner through without caching them or anything.
	if req.UserIsOwner() {
		// Delete the bootstrap cookie so that there won't be two types of auth happening at once.
		http.SetCookie(req.ResponseWriter, &http.Cookie{
			Name:     bootstrap.ObotBootstrapCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   strings.HasPrefix(h.serverURL, "https://"),
		})

		// Redirect to the admin dashboard.
		http.Redirect(
			req.ResponseWriter,
			req.Request,
			"/admin",
			http.StatusFound,
		)

		return nil
	}

	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	// Note: This endpoint does NOT require bootstrap authentication
	// because the OAuth user is calling it after authentication

	// Get the authenticated user info from context
	authProviderUserID := req.AuthProviderUserID()
	if authProviderUserID == "" {
		return types.NewErrHTTP(http.StatusBadRequest,
			"no auth provider user ID in context")
	}

	// Get user by ID
	userID := req.UserID()
	if userID == 0 {
		return types.NewErrHTTP(http.StatusBadRequest, "no user ID in context")
	}

	user, err := req.GatewayClient.UserByID(req.Context(), fmt.Sprintf("%d", userID))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get auth provider info from context
	authProviderName, authProviderNamespace := req.AuthProviderNameAndNamespace()

	// Cache the temporary user
	// This will fail if another user is already cached
	if err := req.GatewayClient.SetTempUserCache(req.Context(), user, authProviderName, authProviderNamespace); err != nil {
		return types.NewErrHTTP(http.StatusConflict, err.Error())
	}

	// Redirect to admin page with success message
	// The UI will then call GET /api/setup/temp-user to display details
	http.Redirect(
		req.ResponseWriter,
		req.Request,
		"/oauth2/sign_out?rd=/admin?setup=complete",
		http.StatusFound,
	)

	return nil
}
