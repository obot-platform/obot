package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/localauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LocalAuthHandler struct {
	provider *localauth.Provider
}

func NewLocalAuthHandler(provider *localauth.Provider) *LocalAuthHandler {
	return &LocalAuthHandler{
		provider: provider,
	}
}

// LocalAuthUser is a user of the local auth provider, as returned by the API.
// Passwords are never returned, in any form.
type LocalAuthUser struct {
	types.Metadata
	Email string `json:"email"`
}

type localAuthUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
			Metadata: types.Metadata{
				ID:      strconv.FormatUint(uint64(user.ID), 10),
				Created: *types.NewTime(user.CreatedAt),
			},
			Email: user.Email,
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

	user, err := h.provider.CreateUser(req.Context(), body.Email, body.Password)
	if errors.Is(err, gateway.ErrLocalAuthUserExists) {
		return types.NewErrBadRequest("a local user with that email already exists")
	} else if invalid := (localauth.InvalidUserError{}); errors.As(err, &invalid) {
		return types.NewErrBadRequest("%s", invalid.Error())
	} else if err != nil {
		return fmt.Errorf("failed to create local auth user: %w", err)
	}

	return req.Write(LocalAuthUser{
		Metadata: types.Metadata{
			ID:      strconv.FormatUint(uint64(user.ID), 10),
			Created: *types.NewTime(user.CreatedAt),
		},
		Email: user.Email,
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

	err = h.provider.SetPassword(req.Context(), id, body.Password)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.NewErrNotFound("local auth user not found")
	} else if invalid := (localauth.InvalidUserError{}); errors.As(err, &invalid) {
		return types.NewErrBadRequest("%s", invalid.Error())
	} else if err != nil {
		return fmt.Errorf("failed to set password for local auth user: %w", err)
	}

	return nil
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
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.UserDeletePrefix,
					Namespace:    req.Namespace(),
				},
				Spec: v1.UserDeleteSpec{
					UserID: gatewayUser.ID,
				},
			}); err != nil {
				return fmt.Errorf("failed to start deletion of user owned objects: %v", err)
			}

			log.Infof("Scheduled user cleanup after deletion: targetUserID=%d", gatewayUser.ID)
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
