package server

import (
	"errors"
	"fmt"
	"net/http"

	types2 "github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/api"
	"github.com/acorn-io/acorn/pkg/gateway/types"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	"gorm.io/gorm"
)

var pkgLog = mvl.Package()

func (s *Server) getCurrentUser(apiContext api.Context) error {
	user, err := s.client.User(apiContext.Context(), apiContext.User.GetName())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The only reason this would happen is if auth is turned off.
		role := types2.RoleBasic
		if apiContext.UserIsAdmin() {
			role = types2.RoleAdmin
		}
		return apiContext.Write(types2.User{
			Username: apiContext.User.GetName(),
			Role:     role,
		})
	} else if err != nil {
		return err
	}

	if err = s.client.UpdateProfileIconIfNeeded(apiContext.Context(), user, apiContext.AuthProviderID()); err != nil {
		pkgLog.Warnf("failed to update profile icon for user %s: %v", user.Username, err)
	}

	return apiContext.Write(types.ConvertUser(user))
}

func (s *Server) getUsers(apiContext api.Context) error {
	userQuery := types.NewUserQuery(apiContext.URL.Query())

	var users []types.User
	if err := s.db.WithContext(apiContext.Context()).Scopes(userQuery.Scope).Find(&users).Error; err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	items := make([]types2.User, 0, len(users))
	for _, user := range users {
		items = append(items, *types.ConvertUser(&user))
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) getUser(apiContext api.Context) error {
	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	user := new(types.User)
	if err := s.db.WithContext(apiContext.Context()).Where("username = ?", username).First(user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", username)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	return apiContext.Write(types.ConvertUser(user))
}

func (s *Server) updateUser(apiContext api.Context) error {
	requestingUsername := apiContext.User.GetName()
	userIsAdmin := apiContext.UserIsAdmin()

	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	if !userIsAdmin && requestingUsername != username {
		return types2.NewErrHttp(http.StatusForbidden, "only admins can update other users")
	}

	user := new(types.User)
	if err := apiContext.Read(user); err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, "invalid user request body")
	}

	existingUser := new(types.User)
	status := http.StatusInternalServerError
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ?", username).First(existingUser).Error; err != nil {
			return err
		}

		// If the username is being changed, then ensure that a user with that name doesn't already exist.
		if user.Username != "" && user.Username != username {
			if err := tx.Model(user).Where("username = ?", user.Username).First(new(types.User)).Error; err == nil {
				status = http.StatusConflict
				return fmt.Errorf("user with username %q already exists", user.Username)
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			existingUser.Username = user.Username
		}

		// Only admins can change user roles.
		if userIsAdmin {
			// If the role is being changed from admin to non-admin, then ensure that this isn't the last admin.
			if user.Role > 0 && existingUser.Role.HasRole(types2.RoleAdmin) && !user.Role.HasRole(types2.RoleAdmin) {
				var adminCount int64
				if err := tx.Model(new(types.User)).Count(&adminCount).Error; err != nil {
					return err
				}

				if adminCount <= 1 {
					status = http.StatusBadRequest
					return fmt.Errorf("cannot remove last admin")
				}
			}

			existingUser.Role = user.Role
		}

		return tx.Updates(existingUser).Error
	}); err != nil {
		return types2.NewErrHttp(status, fmt.Sprintf("failed to update user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser))
}

func (s *Server) deleteUser(apiContext api.Context) error {
	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "username path parameter is required")
	}

	existingUser := new(types.User)
	status := http.StatusInternalServerError
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("username = ?", username).First(existingUser).Error; err != nil {
			return err
		}

		if existingUser.Role.HasRole(types2.RoleAdmin) {
			var adminCount int64
			if err := tx.Model(new(types.User)).Count(&adminCount).Error; err != nil {
				return err
			}

			if adminCount <= 1 {
				status = http.StatusBadRequest
				return fmt.Errorf("cannot remove last admin")
			}
		}

		if err := tx.Where("user_id = ?", existingUser.ID).Delete(new(types.Identity)).Error; err != nil {
			return err
		}

		return tx.Delete(existingUser).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		return types2.NewErrHttp(status, fmt.Sprintf("failed to delete user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser))
}
