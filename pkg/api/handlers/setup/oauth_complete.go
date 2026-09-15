package setup

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/bootstrap"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/system"
)

// OAuthComplete handles the OAuth callback for setup flow.
// This endpoint is called after oauth2-proxy completes authentication.
// The sole local account becomes Owner automatically. Other identities are cached
// for the bootstrap user to review and confirm as the first Owner.
// Endpoint: GET /api/setup/oauth-complete
func (h *Handler) OAuthComplete(req api.Context) error {
	// If the user that just logged in is an Owner, then we can redirect them now.
	// The setup routes will be disabled, so we can just send the owner through without caching them or anything.
	if req.UserIsOwner() {
		slog.Info("Bypassing setup OAuth completion because authenticated user is already owner")
		return h.completeOwnerLogin(req)
	}

	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	// Note: This endpoint does NOT require bootstrap authentication
	// because the OAuth user is calling it after authentication

	// Get the authenticated user info from context
	authProviderUserID := req.AuthProviderUserID()
	if authProviderUserID == "" {
		slog.Info("Rejecting setup OAuth completion due to missing auth provider user ID in context")
		return types.NewErrHTTP(http.StatusBadRequest,
			"no auth provider user ID in context")
	}

	// Get user by ID
	userID := req.UserID()
	if userID == 0 {
		slog.Info("Rejecting setup OAuth completion due to missing user ID in context")
		return types.NewErrHTTP(http.StatusBadRequest, "no user ID in context")
	}

	user, err := req.GatewayClient.UserByID(req.Context(), fmt.Sprintf("%d", userID))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get auth provider info from context
	authProviderName, authProviderNamespace := req.AuthProviderNameAndNamespace()

	if authProviderName == system.LocalAuthProvider && authProviderNamespace == system.DefaultNamespace {
		configured, err := h.authProviderGetter.GetConfiguredAuthProvider(req.Context())
		if err != nil {
			return fmt.Errorf("failed to get configured auth provider: %w", err)
		}

		if configured != system.LocalAuthProvider {
			return types.NewErrHTTP(http.StatusForbidden, "local authentication is not the configured provider")
		}

		localUsers, err := req.GatewayClient.LocalAuthUsers(req.Context())
		if err != nil {
			return fmt.Errorf("failed to get local auth users: %w", err)
		}

		if len(localUsers) == 0 {
			return types.NewErrHTTP(http.StatusForbidden, "no local account exists for setup")
		}

		// Older installations may already have several candidates. Keep their explicit
		// handoff rather than choosing an owner based on who happens to sign in first.
		if len(localUsers) == 1 {
			localUser := localUsers[0]
			if localUser.Email != gateway.NormalizeEmail(authProviderUserID) || localUser.Email != gateway.NormalizeEmail(user.Email) {
				return types.NewErrHTTP(http.StatusForbidden, "sign in with the initial local account to continue setup")
			}

			if localUser.RequirePasswordChange {
				return types.NewErrHTTP(http.StatusForbidden, "change your password before completing setup")
			}

			if err := PromoteToOwner(req, user); err != nil {
				return err
			}

			return h.completeOwnerLogin(req)
		}
	}

	// Cache the temporary user
	// This will fail if another user is already cached
	if err := req.GatewayClient.SetTempUserCache(req.Context(), user, authProviderName, authProviderNamespace); err != nil {
		slog.Info("Rejecting setup OAuth completion because temporary user cache is already occupied", "userID", user.ID)
		return types.NewErrHTTP(http.StatusConflict, err.Error())
	}
	slog.Info("Cached temporary setup user after OAuth completion", "userID", user.ID, "providerNamespace", authProviderNamespace, "providerName", authProviderName)

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

func (h *Handler) completeOwnerLogin(req api.Context) error {
	// Keep the provider session and remove bootstrap authentication from this browser.
	http.SetCookie(req.ResponseWriter, &http.Cookie{
		Name:     bootstrap.ObotBootstrapCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(h.serverURL, "https://"),
	})

	http.Redirect(req.ResponseWriter, req.Request, "/admin", http.StatusFound)
	return nil
}
