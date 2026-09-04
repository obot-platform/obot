package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/auth"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/localauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"gorm.io/gorm"
)

type LocalAuthHandler struct {
	provider *localauth.Provider
}

// LocalAuthUser is a user of the local auth provider, as returned by the API.
// Passwords are never returned, in any form.
type LocalAuthUser struct {
	types.Metadata
	Email                 string `json:"email"`
	RequirePasswordChange bool   `json:"requirePasswordChange"`
}

type localAuthUserRequest struct {
	Email                 string `json:"email"`
	Password              string `json:"password"`
	RequirePasswordChange *bool  `json:"requirePasswordChange,omitempty"`
}

type localAuthActivationRequest struct {
	SetupToken string `json:"setupToken"`
}

func NewLocalAuthHandler(provider *localauth.Provider) *LocalAuthHandler {
	return &LocalAuthHandler{
		provider: provider,
	}
}

func (h *LocalAuthHandler) List(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}

	users, err := h.provider.Users(req.Context())
	if err != nil {
		return fmt.Errorf("failed to list local auth users: %w", err)
	}

	items := make([]LocalAuthUser, 0, len(users))
	for _, user := range users {
		items = append(items, LocalAuthUser{
			ID:                    strconv.FormatUint(uint64(user.ID), 10),
			Created:               *types.NewTime(user.CreatedAt),
			Email:                 user.Email,
			RequirePasswordChange: user.RequirePasswordChange,
		})
	}

	return req.Write(types.List[LocalAuthUser]{Items: items})
}

func (h *LocalAuthHandler) Create(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}

	var body localAuthUserRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	user, err := h.provider.CreateUser(req.Context(), body.Email, body.Password, defaultTrue(body.RequirePasswordChange))
	if errors.Is(err, gateway.ErrLocalAuthUserExists) {
		return types.NewErrBadRequest("a local user with that email already exists")
	} else if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
		return types.NewErrBadRequest("%s", invalid.Error())
	} else if err != nil {
		return fmt.Errorf("failed to create local auth user: %w", err)
	}

	return req.Write(LocalAuthUser{
		ID:                    strconv.FormatUint(uint64(user.ID), 10),
		Created:               *types.NewTime(user.CreatedAt),
		Email:                 user.Email,
		RequirePasswordChange: user.RequirePasswordChange,
	})
}

// SetPassword resets a local user's password, which also signs them out of all their sessions.
func (h *LocalAuthHandler) SetPassword(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}

	id, err := localAuthUserID(req)
	if err != nil {
		return err
	}

	var body localAuthUserRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	err = h.provider.SetPassword(req.Context(), id, body.Password, defaultTrue(body.RequirePasswordChange))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("local auth user not found")
	} else if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
		return types.NewErrBadRequest("%s", invalid.Error())
	} else if err != nil {
		return fmt.Errorf("failed to set password for local auth user: %w", err)
	}

	return nil
}

// Activate validates the initial owner's setup token and establishes a restricted local auth
// session. Invalid, expired, and completed links intentionally get the same response.
func (h *LocalAuthHandler) Activate(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}

	var body localAuthActivationRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body")
	}

	token, expiresAt, err := h.provider.ActivateInitialOwner(req.Context(), body.SetupToken)
	if err != nil {
		// Logged so repeated failures are visible as possible token guessing. The setup token and
		// the owner's email are deliberately left out.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("Rejected an initial local auth owner setup link", "sourceIP", requestinfo.GetSourceIP(req.Request))
		} else {
			slog.Warn("Failed to activate initial local auth owner", "error", err)
		}
		return types.NewErrHTTP(http.StatusUnauthorized, "invalid or expired setup link")
	}

	h.provider.SetSessionCookie(req.ResponseWriter, token, expiresAt)
	return req.Write(map[string]bool{"activated": true})
}

// ChangePassword changes the caller's own local-auth password. It cannot target an email or user
// ID supplied by the caller.
func (h *LocalAuthHandler) ChangePassword(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}
	name, namespace := req.AuthProviderNameAndNamespace()
	if name != system.LocalAuthProvider || namespace != system.DefaultNamespace {
		return types.NewErrBadRequest("the current user is not authenticated with the local provider")
	}
	if utils.FirstSet(req.User.GetExtra()["password_change_required"]...) != "true" {
		return types.NewErrBadRequest("the current local user does not require a password change")
	}

	var body localAuthUserRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	email := utils.FirstSet(req.User.GetExtra()["email"]...)
	localUser, err := req.GatewayClient.LocalAuthUserByEmail(req.Context(), email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("local auth user not found")
	} else if err != nil {
		return fmt.Errorf("failed to find current local auth user: %w", err)
	}

	cookie, err := req.Cookie(auth.ObotAccessTokenCookie)
	if err != nil || cookie.Value == "" {
		return types.NewErrHTTP(http.StatusUnauthorized, "local auth session is required")
	}
	if err := h.provider.ChangePassword(req.Context(), localUser.ID, body.Password, hash.String(cookie.Value)); err != nil {
		if invalid, ok := errors.AsType[localauth.InvalidUserError](err); ok {
			return types.NewErrBadRequest("%s", invalid.Error())
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrHTTP(http.StatusConflict, "password setup has already been completed or the session is no longer valid")
		}
		return fmt.Errorf("failed to change password: %w", err)
	}

	return req.Write(map[string]bool{"changed": true})
}

func (h *LocalAuthHandler) Delete(req api.Context) error {
	if err := h.enabled(); err != nil {
		return err
	}

	id, err := localAuthUserID(req)
	if err != nil {
		return err
	}

	localUser, err := h.provider.GetUser(req.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.NewErrNotFound("local auth user not found")
		}
		return fmt.Errorf("failed to get local auth user: %w", err)
	}

	gatewayUser, err := req.GatewayClient.UserFromProviderUserID(req.Context(), system.DefaultNamespace, system.LocalAuthProvider, localUser.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get user from provider user ID: %w", err)
	}

	if gatewayUser != nil {
		if _, err = req.GatewayClient.DeleteUser(req.Context(), strconv.FormatUint(uint64(gatewayUser.ID), 10)); err != nil {
			status := http.StatusInternalServerError
			if _, ok := errors.AsType[*gateway.LastAdminError](err); ok {
				status = http.StatusBadRequest
			} else if _, ok := errors.AsType[*gateway.LastOwnerError](err); ok {
				status = http.StatusBadRequest
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				// If the error is a record not found error, then someone else already deleted the user while we were processing.
				return types.NewErrHTTP(status, fmt.Sprintf("failed to delete user: %v", err))
			}
		}

		if err == nil {
			if err = req.Create(&v1.UserDelete{
				GenerateName: system.UserDeletePrefix,
				Namespace:    req.Namespace(),
				Spec: v1.UserDeleteSpec{
					UserID: gatewayUser.ID,
				},
			}); err != nil {
				return fmt.Errorf("failed to start deletion of user owned objects: %v", err)
			}

			slog.Info("Scheduled user cleanup after deletion", "targetUserID", gatewayUser.ID)
		}
	}

	if err = h.provider.DeleteUser(req.Context(), id); errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("local auth user not found")
	} else if err != nil {
		return fmt.Errorf("failed to delete local auth user: %w", err)
	}

	return nil
}

func (h *LocalAuthHandler) enabled() error {
	if h.provider == nil {
		return types.NewErrBadRequest("the local auth provider is not available because authentication is disabled")
	}
	return nil
}

func localAuthUserID(req api.Context) (uint, error) {
	id, err := strconv.ParseUint(req.PathValue("id"), 10, 64)
	if err != nil {
		return 0, types.NewErrBadRequest("invalid local auth user ID")
	}
	return uint(id), nil
}

func defaultTrue(value *bool) bool {
	return value == nil || *value
}
