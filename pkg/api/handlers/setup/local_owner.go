package setup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/localauth"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
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
		return types.NewErrHTTP(http.StatusConflict, localized(req, "setup_local_account_exists"))
	} else if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
		return types.NewErrBadRequest("%s", invalid.Localized(requestLocale(req)))
	} else if err != nil {
		return fmt.Errorf("failed to create local owner account: %w", err)
	}

	user, err := h.ensureLocalOwner(req, localUser.Email)
	if err != nil {
		ctx := context.WithoutCancel(req.Context())
		hasOwner, checkErr := req.GatewayClient.HasSignedInOwner(ctx, system.LocalAuthProvider)
		switch {
		case checkErr != nil:
			slog.Error("Kept local owner account after a failed setup because the Owner check failed", "id", localUser.ID, "error", checkErr)
		case hasOwner:
			slog.Warn("Local owner was recorded, but setup did not finish; sending them to sign in", "id", localUser.ID, "error", err)
			return h.writeLocalOwnerSignIn(req, localUser.Email)
		default:
			if deleteErr := h.localAuth.DeleteUser(ctx, localUser.ID); deleteErr != nil {
				slog.Error("Failed to remove local owner account after a failed setup", "id", localUser.ID, "error", deleteErr)
			}
		}
		return err
	}

	token, expiresAt, err := h.localAuth.StartSession(req.Context(), localUser.ID)
	if err != nil {
		slog.Warn("Failed to start a session for the new local owner; sending them to sign in", "userID", user.ID, "error", err)
		return h.writeLocalOwnerSignIn(req, localUser.Email)
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
	existing, err := req.GatewayClient.UserFromProviderUserID(req.Context(), system.DefaultNamespace, system.LocalAuthProvider, email)
	if err == nil && existing.DisabledAt != nil {
		return nil, disabledLocalOwnerError(req, existing.ID)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to look up local owner: %w", err)
	}

	// EnsureIdentityWithRole raises an existing user's role to Owner and creates a new user as Owner, so the
	// returned user already has the Owner role.
	user, err := req.GatewayClient.EnsureIdentityWithRole(
		req.Context(),
		localauth.Identity(email),
		req.UserTimezone(),
		types.RoleOwner,
		// The first Owner must always be creatable, as the bootstrap user is.
		gateway.UserLimit{Unlimited: true},
	)
	if err != nil {
		var lookupErr error
		existing, lookupErr = req.GatewayClient.UserFromProviderUserID(context.WithoutCancel(req.Context()), system.DefaultNamespace, system.LocalAuthProvider, email)
		if lookupErr != nil || !existing.Role.HasRole(types.RoleOwner) {
			return nil, fmt.Errorf("failed to create local owner: %w", err)
		}
		slog.Warn("Local owner was created, but finishing their sign-in failed", "userID", existing.ID, "error", err)
		requestOwnerRoleChange(context.WithoutCancel(req.Context()), req, existing.ID)
		user = existing
	}
	if user.DisabledAt != nil {
		// The user was disabled after the check above.
		return nil, disabledLocalOwnerError(req, user.ID)
	}
	return user, nil
}

// disabledLocalOwnerError rejects an Owner whose email belongs to a disabled user.
func disabledLocalOwnerError(req api.Context, userID uint) error {
	slog.Warn("Rejected local owner creation because the user is disabled", "userID", userID)
	return types.NewErrBadRequest("%s", localized(req, "setup_local_owner_disabled"))
}

// writeLocalOwnerSignIn answers a setup request whose Owner was recorded but whose browser was not signed in. Setup
// is closed by then, so the response sends the owner to the local sign-in page.
func (h *Handler) writeLocalOwnerSignIn(req api.Context, email string) error {
	h.clearBootstrapCookie(req.ResponseWriter)
	return req.Write(CreateLocalOwnerResponse{
		Email:    email,
		Redirect: localauth.LoginPath + "?rd=" + url.QueryEscape("/admin"),
	})
}
