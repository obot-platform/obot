package setup

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// OAuthComplete handles the OAuth callback for setup flow.
// This endpoint is called after oauth2-proxy completes authentication.
// Any authenticated user can be cached - they don't need to be pre-configured
// as an Owner. The bootstrap user will review their details and decide whether
// to confirm them as the first Owner.
// Endpoint: GET /api/setup/oauth-complete
func (h *Handler) OAuthComplete(req api.Context) error {
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
	authProviderName, authProviderNS := req.AuthProviderNameAndNamespace()

	// Cache the temporary user
	// This will fail if another user is already cached
	if err := req.GatewayClient.SetTempUserCache(user, authProviderName, authProviderNS); err != nil {
		return types.NewErrHTTP(http.StatusConflict, err.Error())
	}

	// Redirect to admin page with success message
	// The UI will then call GET /api/setup/temp-user to display details
	http.Redirect(
		req.ResponseWriter,
		req.Request,
		"/admin?setup=complete",
		http.StatusFound,
	)

	return nil
}
