package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/acorn-io/z"
	"github.com/gptscript-ai/otto/pkg/api"
	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/gorm"
)

type usersListResponse struct {
	Users    []types.User `json:"users"`
	Continue *string      `json:"continue,omitempty"`
}

func (s *Server) getCurrentUser(apiContext api.Context) error {
	user := kcontext.GetUser(apiContext.Context())
	writeResponse(apiContext.Context(), kcontext.GetLogger(apiContext.Context()), apiContext.ResponseWriter, user)
	return nil
}

func (s *Server) getUsers(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	userQuery := types.NewUserQuery(apiContext.Context(), apiContext.URL.Query(), logger)

	var users []types.User
	if err := s.db.WithContext(apiContext.Context()).Scopes(userQuery.Scope).Find(&users).Error; err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, err)
		return nil
	}

	var next *string
	if len(users) == userQuery.Limit {
		next = z.Pointer(strconv.FormatInt(int64(users[len(users)-1].ID), 10))
		users = users[:userQuery.Limit-1]
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, usersListResponse{Users: users, Continue: next})
	return nil
}

func (s *Server) getUser(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())

	username := apiContext.PathValue("username")
	if username == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("username path parameter is required"))
		return nil
	}

	user := new(types.User)
	if err := s.db.WithContext(apiContext.Context()).Where("username = ?", username).First(user).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, err)
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, user)
	return nil
}

func (s *Server) updateUser(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	requestingUser := kcontext.GetUser(apiContext.Context())

	username := apiContext.PathValue("username")
	if username == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("username path parameter is required"))
		return nil
	}

	if !requestingUser.Role.HasRole(types.RoleAdmin) && requestingUser.Username != username {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusForbidden, errors.New("only admins can update other users"))
		return nil
	}

	user := new(types.User)
	if err := apiContext.Read(user); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid user request body: %v", err))
		return nil
	}

	status := http.StatusInternalServerError
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		existingUser := new(types.User)
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
		if requestingUser.Role.HasRole(types.RoleAdmin) {
			// If the role is being changed from admin to non-admin, then ensure that this isn't the last admin.
			if user.Role > 0 && existingUser.Role.HasRole(types.RoleAdmin) && !user.Role.HasRole(types.RoleAdmin) {
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
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to update user: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, user)
	return nil
}

func (s *Server) deleteUser(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())

	username := apiContext.PathValue("username")
	if username == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("username path parameter is required"))
		return nil
	}

	status := http.StatusInternalServerError
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		existingUser := new(types.User)
		if err := tx.Where("username = ?", username).Or("username = ?", username).First(existingUser).Error; err != nil {
			return err
		}

		if existingUser.Role.HasRole(types.RoleAdmin) {
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

		return tx.Where("username = ?", username).Delete(new(types.User)).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to delete user by username", "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to delete user: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"deleted": true})
	return nil
}
