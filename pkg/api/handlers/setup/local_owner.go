package setup

import (
	"fmt"
	"log/slog"
	"mime"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/system"
)

// oauthCompletePath is the API-relative path that finishes owner setup once the browser holds a
// Local session.
const oauthCompletePath = "/setup/oauth-complete"

type CreateLocalOwnerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateLocalOwnerResponse struct {
	Email       string `json:"email"`
	RedirectURL string `json:"redirectUrl"`
}

// CreateLocalOwner creates the initial Local account and signs this browser in as it, so the
// bootstrap user does not have to re-enter the password it just chose. The bootstrap user already
// has full administrative access, and the session is only ever issued for the account created by
// this request. The browser finishes setup at the oauth-complete endpoint, which promotes the account to
// Owner and removes bootstrap authentication.
// Endpoint: POST /api/setup/local-owner
func (h *Handler) CreateLocalOwner(req api.Context) error {
	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	// The bootstrap credential is a cookie, so refuse cross-site requests that would otherwise
	// create the owner account with a password chosen by another site.
	if !localauth.SameOrigin(req.Request) {
		slog.Warn("Rejected cross-origin initial local owner creation")
		return types.NewErrHTTP(http.StatusForbidden, localized(req, "local_login_cross_origin"))
	}
	// SameOrigin allows requests without Origin or Referer, so also require a JSON body: a
	// cross-site form or simple request cannot send one without a CORS preflight.
	if mediaType, _, _ := mime.ParseMediaType(req.Request.Header.Get("Content-Type")); mediaType != "application/json" {
		return types.NewErrHTTP(http.StatusUnsupportedMediaType, localized(req, "local_json_required"))
	}

	if h.localAuth == nil {
		return types.NewErrBadRequest("%s", localized(req, "local_unavailable"))
	}

	configured, err := h.authProviderGetter.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configured != system.LocalAuthProvider {
		return types.NewErrHTTP(http.StatusForbidden, localized(req, "setup_local_not_configured"))
	}

	var body CreateLocalOwnerRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("%s", localized(req, "local_invalid_body", "detail", err.Error()))
	}

	user, err := h.localAuth.CreateUser(req.Context(), body.Email, body.Password, false, true)
	if err != nil {
		return localauth.CreateUserHTTPError(err, requestLocale(req))
	}

	token, expiresAt, err := h.localAuth.StartSession(req.Context(), user.ID)
	if err != nil {
		// The account exists, so the user can still finish setup by signing in.
		return fmt.Errorf("failed to sign in initial local auth user: %w", err)
	}
	h.localAuth.SetSessionCookie(req.ResponseWriter, token, expiresAt)
	slog.Info("Created and signed in initial local auth user during setup", "localUserID", user.ID)

	return req.Write(CreateLocalOwnerResponse{
		Email:       user.Email,
		RedirectURL: req.APIBaseURL + oauthCompletePath,
	})
}
