package setup

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/system"
)

type CreateLocalOwnerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateLocalOwnerResponse struct {
	Email    string `json:"email"`
	Redirect string `json:"redirect"`
}

// CreateLocalOwner creates the first local account, makes it the Owner, and signs this browser in as
// that account, so the bootstrap user does not have to sign in again to finish setup. Because the
// Owner's identity is recorded as signed in, setup closes as soon as this succeeds.
// Endpoint: POST /api/setup/local-owner
func (h *Handler) CreateLocalOwner(req api.Context) error {
	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	if h.localAuth == nil {
		return types.NewErrBadRequest("%s", localized(req, "local_unavailable"))
	}

	// The proxy validates sessions only against the configured provider, so a local session is
	// useless unless local authentication is the configured provider.
	configured, err := h.authProviderGetter.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configured != system.LocalAuthProvider {
		return types.NewErrHTTP(http.StatusPreconditionFailed, localized(req, "setup_local_not_configured"))
	}

	var body CreateLocalOwnerRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("%s", localized(req, "local_invalid_body", "detail", err.Error()))
	}

	// Bootstrap creation succeeds only while no local account exists, under the user creation lock,
	// so concurrent submissions create one account at most.
	localUser, err := h.localAuth.CreateUser(req.Context(), body.Email, body.Password, false, true)
	if errors.Is(err, gateway.ErrBootstrapLocalAuthUserLimit) || errors.Is(err, gateway.ErrLocalAuthUserExists) {
		slog.Info("Rejected local owner creation because a local account already exists")
		return types.NewErrHTTP(http.StatusConflict, localized(req, "setup_owner_exists"))
	} else if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
		locale := "en"
		if req.Request != nil {
			locale = i18n.FromContext(req.Context())
		}
		return types.NewErrBadRequest("%s", invalid.Localized(locale))
	} else if err != nil {
		return fmt.Errorf("failed to create local owner account: %w", err)
	}

	user, err := h.ensureLocalOwner(req, localUser.Email)
	if err != nil {
		// Without an Owner, the account would only block another attempt at the bootstrap limit.
		if deleteErr := h.localAuth.DeleteUser(req.Context(), localUser.ID); deleteErr != nil {
			slog.Error("Failed to remove local owner account after a failed setup", "id", localUser.ID, "error", deleteErr)
		}
		return err
	}

	token, expiresAt, err := h.localAuth.StartSession(req.Context(), localUser.ID)
	if err != nil {
		// The Owner exists and setup is closed, so the account must stay. The owner can sign in normally.
		return err
	}
	h.localAuth.SetSessionCookie(req.ResponseWriter, token, expiresAt)
	h.clearBootstrapCookie(req.ResponseWriter)
	slog.Info("Created local owner during setup", "userID", user.ID)

	return req.Write(CreateLocalOwnerResponse{
		Email:    localUser.Email,
		Redirect: "/admin",
	})
}

// ensureLocalOwner records the local account's identity as a signed-in Owner, which is what its first
// sign-in would otherwise do. The identity and user are committed before the rest of the work, so when
// that work fails after an Owner was recorded, the Owner is kept: deleting the account behind it would
// close setup with nobody able to sign in.
func (h *Handler) ensureLocalOwner(req api.Context, email string) (*gatewaytypes.User, error) {
	user, err := req.GatewayClient.EnsureIdentityWithRole(
		req.Context(),
		localauth.Identity(email),
		req.Request.Header.Get("X-Obot-User-Timezone"),
		types.RoleOwner,
		// The first Owner must always be creatable, as the bootstrap user is.
		gateway.UserLimit{Unlimited: true},
	)
	if err != nil {
		existing, lookupErr := req.GatewayClient.UserFromProviderUserID(req.Context(), system.DefaultNamespace, system.LocalAuthProvider, email)
		if lookupErr != nil || !existing.Role.HasRole(types.RoleOwner) {
			return nil, fmt.Errorf("failed to create local owner: %w", err)
		}
		slog.Warn("Local owner was created, but finishing their sign-in failed", "userID", existing.ID, "error", err)
		return existing, nil
	}

	if err := PromoteToOwner(req, user); err != nil {
		return nil, err
	}
	return user, nil
}
