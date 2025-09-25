package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/mvl"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/proxy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var pkgLog = mvl.Package()

func (s *Server) getCurrentUser(apiContext api.Context) error {
	user, err := apiContext.GatewayClient.User(apiContext.Context(), apiContext.User.GetName())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// This shouldn't happen, but, if it does, then the user would be unauthorized because we can't identify them.
		return types2.NewErrHTTP(http.StatusUnauthorized, "unauthorized")
	} else if err != nil {
		return err
	}

	name, namespace := apiContext.AuthProviderNameAndNamespace()

	if name != "" && namespace != "" {
		providerURL, err := s.dispatcher.URLForAuthProvider(apiContext.Context(), namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get auth provider URL: %v", err)
		}
		if err = apiContext.GatewayClient.UpdateProfileIfNeeded(apiContext.Context(), user, name, namespace, providerURL.String()); err != nil {
			pkgLog.Warnf("failed to update profile icon for user %s: %v", user.Username, err)
		}
	}

	return apiContext.Write(types.ConvertUser(user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, name))
}

func (s *Server) getUsers(apiContext api.Context) error {
	users, err := apiContext.GatewayClient.Users(apiContext.Context(), types.NewUserQuery(apiContext.URL.Query()))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	items := make([]types2.User, 0, len(users))
	for _, user := range users {
		if user.Username != "bootstrap" && user.Email != "" { // Filter out the bootstrap admin
			items = append(items, *types.ConvertUser(&user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, ""))
		}
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) encryptAllUsersAndIdentities(apiContext api.Context) error {
	force := apiContext.URL.Query().Get("force") == "true"

	users, err := apiContext.GatewayClient.Users(apiContext.Context(), types.NewUserQuery(apiContext.URL.Query()))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	for _, user := range users {
		if force || !user.Encrypted {
			if _, err = apiContext.GatewayClient.UpdateUser(apiContext.Context(), apiContext.UserIsAdmin(), &user, strconv.FormatUint(uint64(user.ID), 10)); err != nil {
				return fmt.Errorf("failed to encrypt user with id %d: %v", user.ID, err)
			}
		}
	}

	if err = apiContext.GatewayClient.EncryptIdentities(apiContext.Context(), force); err != nil {
		return fmt.Errorf("failed to encrypt identities: %v", err)
	}

	return apiContext.Write("done")
}

func (s *Server) getUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")

	if userID == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "user_id path parameter is required")
	}

	user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", userID)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	return apiContext.Write(types.ConvertUser(user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, ""))
}

func (s *Server) updateUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	if userID == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "user_id path parameter is required")
	}

	user := new(types.User)
	if err := apiContext.Read(user); err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, "invalid user request body")
	}

	if user.Timezone != "" {
		if _, err := time.LoadLocation(user.Timezone); err != nil {
			return types2.NewErrHTTP(http.StatusBadRequest, "invalid timezone")
		}
	}

	originalUser, err := apiContext.GatewayClient.UserByID(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrHTTP(http.StatusNotFound, "user not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get original user: %v", err))
	}

	if !apiContext.UserIsOwner() {
		if originalUser.Role.HasRole(types2.RoleOwner) != user.Role.HasRole(types2.RoleOwner) {
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can add or remove owner role")
		}
		if originalUser.Role.HasRole(types2.RoleAuditor) != user.Role.HasRole(types2.RoleAuditor) {
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can remove admin role")
		}
	}

	status := http.StatusInternalServerError
	existingUser, err := apiContext.GatewayClient.UpdateUser(apiContext.Context(), apiContext.UserIsAdmin(), user, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		} else if ea := (*client.ExplicitRoleError)(nil); errors.As(err, &ea) {
			status = http.StatusBadRequest
		} else if ae := (*client.AlreadyExistsError)(nil); errors.As(err, &ae) {
			status = http.StatusConflict
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to update user: %v", err))
	}

	if originalUser.Role != existingUser.Role {
		if err = apiContext.Create(&v1.UserRoleChange{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.UserRoleChangePrefix,
				Namespace:    apiContext.Namespace(),
			},
			Spec: v1.UserRoleChangeSpec{
				UserID:  existingUser.ID,
				OldRole: originalUser.Role,
				NewRole: existingUser.Role,
			},
		}); err != nil {
			return fmt.Errorf("failed to create user role change event: %v", err)
		}
	}

	return apiContext.Write(types.ConvertUser(existingUser, apiContext.GatewayClient.HasExplicitRole(existingUser.Email) != types2.RoleUnknown, ""))
}

func (s *Server) markUserInternal(apiContext api.Context) error {
	return s.changeUserInternalStatus(apiContext, true)
}

func (s *Server) markUserExternal(apiContext api.Context) error {
	return s.changeUserInternalStatus(apiContext, false)
}

func (s *Server) changeUserInternalStatus(apiContext api.Context, internal bool) error {
	userID := apiContext.PathValue("user_id")
	if userID == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "user_id path parameter is required")
	}

	if err := apiContext.GatewayClient.UpdateUserInternalStatus(apiContext.Context(), userID, internal); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", userID)
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to update user: %v", err))
	}

	return nil
}

func (s *Server) deleteUser(apiContext api.Context) (err error) {
	userID := apiContext.PathValue("user_id")
	isDeleteMe := userID == ""
	if isDeleteMe {
		// This is the "delete me" API
		userID = apiContext.User.GetUID()
	}

	existingUser, err := apiContext.GatewayClient.UserByID(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", userID)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	if !apiContext.UserIsOwner() {
		if existingUser.Role.HasRole(types2.RoleOwner) {
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can delete an owner")
		}
		if existingUser.Role.HasRole(types2.RoleAuditor) {
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can delete an auditor")
		}
	}

	status := http.StatusInternalServerError
	_, err = apiContext.GatewayClient.DeleteUser(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to delete user: %v", err))
	}

	if err = apiContext.Create(&v1.UserDelete{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.UserDeletePrefix,
			Namespace:    apiContext.Namespace(),
		},
		Spec: v1.UserDeleteSpec{
			UserID: existingUser.ID,
		},
	}); err != nil {
		return fmt.Errorf("failed to start deletion of user owned objects: %v", err)
	}

	// Only clear the cookie if this is a "delete me" operation
	if isDeleteMe {
		// Tell the browser to remove the access token cookie, so that the user does not immediately attempt to authenticate again.
		http.SetCookie(apiContext.ResponseWriter, &http.Cookie{
			Name:     proxy.ObotAccessTokenCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   strings.HasPrefix(s.uiURL, "https://"),
		})
	}

	return apiContext.Write(types.ConvertUser(existingUser, apiContext.GatewayClient.HasExplicitRole(existingUser.Email) != types2.RoleUnknown, ""))
}

func (s *Server) listAuthGroups(apiContext api.Context) error {
	name, namespace := apiContext.AuthProviderNameAndNamespace()

	if name != "" && namespace != "" {
		providerURL, err := s.dispatcher.URLForAuthProvider(apiContext.Context(), namespace, name)
		if err != nil {
			return fmt.Errorf("failed to get auth provider URL: %v", err)
		}
		groups, err := apiContext.GatewayClient.ListAuthGroups(
			apiContext.Context(),
			providerURL.String(),
			namespace,
			name,
			apiContext.URL.Query().Get("name"),
		)
		if err != nil {
			return fmt.Errorf("failed to list auth groups: %v", err)
		}
		return apiContext.Write(groups)
	}

	return apiContext.Write([]types.Group{})
}
