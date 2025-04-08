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
			return fmt.Errorf("failmed to get auth provider URL: %v", err)
		}
		if err = apiContext.GatewayClient.UpdateProfileIconIfNeeded(apiContext.Context(), user, name, namespace, providerURL.String()); err != nil {
			pkgLog.Warnf("failed to update profile icon for user %s: %v", user.Username, err)
		}
	}

	return apiContext.Write(types.ConvertUser(user, apiContext.GatewayClient.IsExplicitAdmin(user.Email), name))
}

func (s *Server) getUsers(apiContext api.Context) error {
	users, err := apiContext.GatewayClient.Users(apiContext.Context(), types.NewUserQuery(apiContext.URL.Query()))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	items := make([]types2.User, 0, len(users))
	for _, user := range users {
		if user.Username != "bootstrap" && user.Email != "" { // Filter out the bootstrap admin
			items = append(items, *types.ConvertUser(&user, apiContext.GatewayClient.IsExplicitAdmin(user.Email), ""))
		}
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) getUser(apiContext api.Context) error {
	var (
		getByID      = apiContext.URL.Query().Get("by-id") == "true"
		usernameOrID = apiContext.PathValue("username_or_id")
		user         *types.User
	)

	if usernameOrID == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "username path parameter is required")
	}

	var err error
	if getByID {
		user, err = apiContext.GatewayClient.UserByID(apiContext.Context(), usernameOrID)
	} else {
		user, err = apiContext.GatewayClient.User(apiContext.Context(), usernameOrID)
	}

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", usernameOrID)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	return apiContext.Write(types.ConvertUser(user, apiContext.GatewayClient.IsExplicitAdmin(user.Email), ""))
}

func (s *Server) updateUser(apiContext api.Context) error {
	requestingUsername := apiContext.User.GetName()
	actingUserIsAdmin := apiContext.UserIsAdmin()

	username := apiContext.PathValue("username")
	if username == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "username path parameter is required")
	}

	if !actingUserIsAdmin && requestingUsername != username {
		return types2.NewErrHTTP(http.StatusForbidden, "only admins can update other users")
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

	status := http.StatusInternalServerError
	existingUser, err := apiContext.GatewayClient.UpdateUser(apiContext.Context(), actingUserIsAdmin, user, username)
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
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to update user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser, apiContext.GatewayClient.IsExplicitAdmin(existingUser.Email), ""))
}

func (s *Server) deleteUser(apiContext api.Context) error {
	username := apiContext.PathValue("username")
	if username == "" {
		// This is the "delete me" API
		username = apiContext.User.GetName()
	}

	existingUser, err := apiContext.GatewayClient.User(apiContext.Context(), username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", username)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	if err = apiContext.Create(&v1.UserDelete{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.UserDeletePrefix,
			Namespace: apiContext.Namespace(),
		},
		Spec: v1.UserDeleteSpec{
			UserID: existingUser.ID,
		},
	}); err != nil {
		return fmt.Errorf("failed to start deletion of user owned objects: %v", err)
	}

	status := http.StatusInternalServerError
	_, err = apiContext.GatewayClient.DeleteUser(apiContext.Context(), username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to delete user: %v", err))
	}

	return apiContext.Write(types.ConvertUser(existingUser, apiContext.GatewayClient.IsExplicitAdmin(existingUser.Email), ""))
}
