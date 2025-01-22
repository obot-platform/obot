package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/mvl"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

var pkgLog = mvl.Package()

func (s *Server) getCurrentUser(apiContext api.Context) error {
	user, err := s.client.User(apiContext.Context(), apiContext.User.GetName())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// This shouldn't happen, but, if it does, then the user would be unauthorized because we can't identify them.
		return types2.NewErrHttp(http.StatusUnauthorized, "unauthorized")
	} else if err != nil {
		return err
	}

	name, namespace := apiContext.AuthProviderNameAndNamespace()

	if name != "" && namespace != "" {
		providerURL, err := s.dispatcher.URLForAuthProvider(apiContext.Context(), namespace, name)
		if err != nil {
			return fmt.Errorf("failmed to get auth provider URL: %v", err)
		}
		if err = s.client.UpdateProfileIconIfNeeded(apiContext.Context(), user, name, namespace, providerURL.String()); err != nil {
			pkgLog.Warnf("failed to update profile icon for user %s: %v", user.Username, err)
		}
	}

	return apiContext.Write(types.ConvertUser(user, s.client.IsExplicitAdmin(user.Email)))
}

func (s *Server) getUsers(apiContext api.Context) error {
	users, err := s.client.Users(apiContext.Context(), types.NewUserQuery(apiContext.URL.Query()))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	items := make([]types2.User, 0, len(users))
	for _, user := range users {
		items = append(items, *types.ConvertUser(&user, s.client.IsExplicitAdmin(user.Email)))
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) getUser(apiContext api.Context) error {
	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	user, err := s.client.User(apiContext.Context(), username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", username)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	return apiContext.Write(types.ConvertUser(user, s.client.IsExplicitAdmin(user.Email)))
}

func (s *Server) updateUser(apiContext api.Context) error {
	requestingUsername := apiContext.User.GetName()
	actingUserIsAdmin := apiContext.UserIsAdmin()

	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	if !actingUserIsAdmin && requestingUsername != username {
		return types2.NewErrHttp(http.StatusForbidden, "only admins can update other users")
	}

	user := new(types.User)
	if err := apiContext.Read(user); err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, "invalid user request body")
	}

	if user.Timezone != "" {
		if _, err := time.LoadLocation(user.Timezone); err != nil {
			return types2.NewErrHttp(http.StatusBadRequest, "invalid timezone")
		}
	}

	status := http.StatusInternalServerError
	existingUser, err := s.client.UpdateUser(apiContext.Context(), actingUserIsAdmin, user, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		} else if ea := (*client.ExplicitAdminError)(nil); errors.As(err, &ea) {
			status = http.StatusBadRequest
		} else if ae := (*client.AlreadyExistsError)(nil); errors.As(err, &ae) {
			status = http.StatusConflict
		}
		return types2.NewErrHttp(status, fmt.Sprintf("failed to update user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser, s.client.IsExplicitAdmin(existingUser.Email)))
}

func (s *Server) deleteUser(apiContext api.Context) error {
	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	status := http.StatusInternalServerError
	existingUser, err := s.client.DeleteUser(apiContext.Context(), username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		}
		return types2.NewErrHttp(status, fmt.Sprintf("failed to delete user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser, s.client.IsExplicitAdmin(existingUser.Email)))
}
