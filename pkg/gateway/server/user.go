package server

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/proxy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	maxGroupIDsPerRequest = 100
)

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
			return fmt.Errorf("failed to get auth provider URL: %w", err)
		}
		if err = apiContext.GatewayClient.UpdateProfileIfNeeded(apiContext.Context(), user, name, namespace, providerURL.String()); err != nil {
			slog.Warn("failed to update profile icon for user", "username", user.Username, "error", err)
		}
	}

	// Get user's auth groups and compute effective role
	authGroupStrs := apiContext.User.GetExtra()["auth_provider_groups"]
	effectiveRole, err := apiContext.GatewayClient.ResolveUserEffectiveRole(apiContext.Context(), user, authGroupStrs)
	if err != nil {
		slog.Warn("failed to resolve effective role for user", "username", user.Username, "error", err)
		effectiveRole = user.Role
	}

	result := types.ConvertUserWithEffectiveRole(user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, name, effectiveRole)
	result.AuthProviderGroups = authGroupStrs
	result.RequirePasswordChange = cmp.Or(apiContext.User.GetExtra()["password_change_required"]...) == "true"
	return apiContext.Write(result)
}

func (s *Server) getUsers(apiContext api.Context) error {
	users, err := apiContext.GatewayClient.Users(apiContext.Context(), types.NewUserQuery(apiContext.URL.Query()))
	if err != nil {
		return fmt.Errorf("failed to get users: %v", err)
	}

	// Filter out bootstrap user and collect valid users with their IDs
	validUsers := make([]types.User, 0, len(users))
	userIDs := make([]uint, 0, len(users))
	for _, user := range users {
		if user.Username != system.BootstrapName && user.Email != "" {
			validUsers = append(validUsers, user)
			userIDs = append(userIDs, user.ID)
		}
	}

	// Basic and Power users are only allowed to access IDs, display names, and statuses, so we have all the information
	// needed for that.
	if userIsBasicOrPower(apiContext.User) {
		trimmedUsers := make([]types2.User, 0, len(validUsers))
		for _, u := range validUsers {
			trimmedUsers = append(trimmedUsers, trimUser(u))
		}
		return apiContext.Write(types2.UserList{Items: trimmedUsers})
	}

	// Bulk fetch group memberships for all users (single query)
	userGroupMemberships, err := apiContext.GatewayClient.GetUserGroupMemberships(apiContext.Context(), userIDs)
	if err != nil {
		return fmt.Errorf("failed to get user group memberships: %v", err)
	}

	// Bulk compute effective roles for all users (single query)
	effectiveRoles, err := apiContext.GatewayClient.ResolveUserEffectiveRolesBulk(apiContext.Context(), validUsers, userGroupMemberships)
	if err != nil {
		return fmt.Errorf("failed to resolve effective roles: %v", err)
	}

	managed, err := apiContext.GatewayClient.SCIMManagedUsers(apiContext.Context(), userIDs)
	if err != nil {
		return fmt.Errorf("failed to get SCIM-provisioned users: %v", err)
	}

	// Build response with computed effective roles
	items := make([]types2.User, 0, len(validUsers))
	for _, user := range validUsers {
		effectiveRole := user.Role
		if role, ok := effectiveRoles[user.ID]; ok {
			effectiveRole = role
		}

		result := types.ConvertUserWithEffectiveRole(&user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, "", effectiveRole)
		result.AuthProviderGroups = userGroupMemberships[user.ID]
		setSCIMManagement(result, user.ID, managed)
		items = append(items, *result)
	}

	return apiContext.Write(types2.UserList{Items: items})
}

func (s *Server) encryptAllUsersAndIdentities(apiContext api.Context) error {
	force := apiContext.URL.Query().Get("force") == "true"

	if err := apiContext.GatewayClient.EncryptUsers(apiContext.Context(), force); err != nil {
		return fmt.Errorf("failed to encrypt users: %v", err)
	}

	if err := apiContext.GatewayClient.EncryptIdentities(apiContext.Context(), force); err != nil {
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

	// Basic and Power users are only allowed to access IDs, display names, and statuses.
	if userIsBasicOrPower(apiContext.User) {
		return apiContext.Write(trimUser(*user))
	}

	// Get user's groups and compute effective role
	groupIDs, err := apiContext.GatewayClient.ListGroupIDsForUser(apiContext.Context(), user.ID)
	if err != nil {
		slog.Warn("failed to get groups for user", "username", user.Username, "error", err)
		groupIDs = nil
	}

	effectiveRole, err := apiContext.GatewayClient.ResolveUserEffectiveRole(apiContext.Context(), user, groupIDs)
	if err != nil {
		slog.Warn("failed to resolve effective role for user", "username", user.Username, "error", err)
		effectiveRole = user.Role
	}

	managed, err := apiContext.GatewayClient.SCIMManagedUsers(apiContext.Context(), []uint{user.ID})
	if err != nil {
		return fmt.Errorf("failed to check the SCIM binding of user: %v", err)
	}

	result := types.ConvertUserWithEffectiveRole(user, apiContext.GatewayClient.HasExplicitRole(user.Email) != types2.RoleUnknown, "", effectiveRole)
	result.AuthProviderGroups = groupIDs
	setSCIMManagement(result, user.ID, managed)
	return apiContext.Write(result)
}

func (s *Server) updateUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	if userID == "" {
		// This is a request to /api/me
		userID = apiContext.User.GetUID()
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
			slog.Info("Denied user role update", "targetUserID", userID, "reason", "owner_role_change_requires_owner")
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can add or remove owner role")
		}
		if originalUser.Role.HasRole(types2.RoleAuditor) != user.Role.HasRole(types2.RoleAuditor) {
			slog.Info("Denied user role update", "targetUserID", userID, "reason", "auditor_role_change_requires_owner")
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can add or remove auditor role")
		}
	}

	status := http.StatusInternalServerError
	existingUser, err := apiContext.GatewayClient.UpdateUser(apiContext.Context(), apiContext.UserIsAdmin(), user, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if lae := (*client.LastAdminError)(nil); errors.As(err, &lae) {
			status = http.StatusBadRequest
		} else if loe := (*client.LastOwnerError)(nil); errors.As(err, &loe) {
			status = http.StatusBadRequest
		} else if ea := (*client.ExplicitRoleError)(nil); errors.As(err, &ea) {
			status = http.StatusBadRequest
		} else if ae := (*client.AlreadyExistsError)(nil); errors.As(err, &ae) {
			status = http.StatusConflict
		} else if _, ok := errors.AsType[*client.SCIMManagedUserError](err); ok {
			// As for a deletion: the identity provider manages the user.
			status = http.StatusConflict
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to update user: %v", err))
	}

	// Create UserRoleChange event to trigger reconciliation if personal role changed
	if originalUser.Role != existingUser.Role {
		slog.Info("User role changed via API", "userID", existingUser.ID, "oldRole", originalUser.Role, "newRole", existingUser.Role)
		if err = apiContext.Create(&v1.UserRoleChange{
			GenerateName: system.UserRoleChangePrefix,
			Namespace:    apiContext.Namespace(),
			Spec: v1.UserRoleChangeSpec{
				UserID: existingUser.ID,
			},
		}); err != nil {
			return fmt.Errorf("failed to create user role change event: %v", err)
		}
	}
	slog.Info("Updated user profile via API", "userID", existingUser.ID)

	managed, err := apiContext.GatewayClient.SCIMManagedUsers(apiContext.Context(), []uint{existingUser.ID})
	if err != nil {
		return fmt.Errorf("failed to check the SCIM binding of user: %v", err)
	}

	converted := types.ConvertUser(existingUser, apiContext.GatewayClient.HasExplicitRole(existingUser.Email) != types2.RoleUnknown, "")
	setSCIMManagement(converted, existingUser.ID, managed)
	return apiContext.Write(converted)
}

// setSCIMManagement reports the user with userID as managed by SCIM if SCIMManagedUsers returned them in managed.
func setSCIMManagement(user *types2.User, userID uint, managed map[uint]struct{}) {
	if _, ok := managed[userID]; ok {
		user.ManagementSource = types2.UserManagementSourceSCIM
	}
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

// POST /api/users/{user_id}/enable
// Enables a disabled user, restoring their access with the same account and data. It is refused with 409 while SCIM
// manages the user, whose identity provider decides that, and only an Owner can enable a user who has, through their
// own role, a group, or an explicit role of their email, the Owner, auditor, or user impersonation role.
func (s *Server) enableUser(apiContext api.Context) error {
	userID := apiContext.PathValue("user_id")
	existingUser, err := apiContext.GatewayClient.UserByID(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", userID)
		}
		return fmt.Errorf("failed to get user: %v", err)
	}

	if !apiContext.UserIsOwner() {
		// Enabling restores every role the user gets: their own, those of their groups, and an explicit role of their
		// email.
		groupIDs, err := apiContext.GatewayClient.ListGroupIDsForUser(apiContext.Context(), existingUser.ID)
		if err != nil {
			return fmt.Errorf("failed to list the groups of user: %v", err)
		}
		role, err := apiContext.GatewayClient.ResolveUserEffectiveRole(apiContext.Context(), existingUser, groupIDs)
		if err != nil {
			return fmt.Errorf("failed to resolve the role of user: %v", err)
		}
		role |= apiContext.GatewayClient.HasExplicitRole(existingUser.Email)
		if role.HasRole(types2.RoleOwner) || role.HasRole(types2.RoleAuditor) {
			slog.Info("Denied enabling user", "targetUserID", userID, "reason", "privileged_user_enable_requires_owner")
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can enable an owner or an auditor")
		}
	}

	enabled, err := apiContext.GatewayClient.EnableUser(apiContext.Context(), existingUser.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("user %s not found", userID)
		} else if _, ok := errors.AsType[*client.UserDeletedError](err); ok {
			return types2.NewErrNotFound("user %s not found", userID)
		} else if managed, ok := errors.AsType[*client.SCIMManagedUserError](err); ok {
			return types2.NewErrHTTP(http.StatusConflict, managed.Message)
		}
		return fmt.Errorf("failed to enable user: %v", err)
	}
	slog.Info("Enabled user via API", "targetUserID", enabled.ID)

	managed, err := apiContext.GatewayClient.SCIMManagedUsers(apiContext.Context(), []uint{enabled.ID})
	if err != nil {
		return fmt.Errorf("failed to check the SCIM binding of user: %v", err)
	}
	converted := types.ConvertUser(enabled, apiContext.GatewayClient.HasExplicitRole(enabled.Email) != types2.RoleUnknown, "")
	setSCIMManagement(converted, enabled.ID, managed)
	return apiContext.Write(converted)
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

	if isDeleteMe {
		// The identity provider controls the accounts that SCIM has provisioned, so their owners cannot delete them.
		binding, err := apiContext.GatewayClient.SCIMUserBindingForUser(apiContext.Context(), existingUser.ID)
		if err != nil {
			return fmt.Errorf("failed to check the SCIM binding of user: %w", err)
		}
		if binding != nil {
			slog.Info("Denied user deletion", "targetUserID", userID, "reason", "scim_managed_self_delete")
			return types2.NewErrHTTP(http.StatusForbidden, "your account is managed by your identity provider; ask your administrator to remove your assignment there")
		}
	}

	if !apiContext.UserIsOwner() {
		if existingUser.Role.HasRole(types2.RoleOwner) {
			slog.Info("Denied user deletion", "targetUserID", userID, "reason", "owner_delete_requires_owner")
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can delete an owner")
		}
		if existingUser.Role.HasRole(types2.RoleAuditor) {
			slog.Info("Denied user deletion", "targetUserID", userID, "reason", "auditor_delete_requires_owner")
			return types2.NewErrHTTP(http.StatusForbidden, "only owner can delete an auditor")
		}
	}

	status := http.StatusInternalServerError
	err = apiContext.GatewayClient.DeleteUser(apiContext.Context(), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		} else if _, ok := errors.AsType[*client.LastAdminError](err); ok {
			status = http.StatusBadRequest
		} else if _, ok := errors.AsType[*client.LastOwnerError](err); ok {
			status = http.StatusBadRequest
		} else if _, ok := errors.AsType[*client.SCIMManagedUserError](err); ok {
			status = http.StatusConflict
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("failed to delete user: %v", err))
	}

	if err = apiContext.Create(&v1.UserDelete{
		GenerateName: system.UserDeletePrefix,
		Namespace:    apiContext.Namespace(),
		Spec: v1.UserDeleteSpec{
			UserID: existingUser.ID,
		},
	}); err != nil {
		return fmt.Errorf("failed to start deletion of user owned objects: %v", err)
	}
	slog.Info("Scheduled user cleanup after deletion", "targetUserID", existingUser.ID, "deleteMe", isDeleteMe)

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

// GET /api/groups?name=&limit=&cursor=&ids=
// Returns one page of the auth provider's groups, optionally filtered by name. Paging is by opaque
// cursor: the response carries the position of the next page, or nothing when there are no more.
// Passing ids instead resolves those specific group IDs to their display names.
func (s *Server) listAuthGroups(apiContext api.Context) error {
	name, namespace := apiContext.AuthProviderNameAndNamespace()
	if name == "" || namespace == "" {
		return apiContext.Write(types.GroupListResponse{
			Items:  []types.Group{},
			Source: types.GroupSourceCache,
		})
	}

	query := apiContext.URL.Query()

	providerURL, err := s.dispatcher.URLForAuthProvider(apiContext.Context(), namespace, name)
	if err != nil {
		return fmt.Errorf("failed to get auth provider URL: %w", err)
	}

	if rawIDs := query.Get("ids"); rawIDs != "" {
		ids, err := splitGroupIDs(rawIDs)
		if err != nil {
			return types2.NewErrHTTP(http.StatusBadRequest, err.Error())
		}

		groups, err := apiContext.GatewayClient.ResolveAuthGroups(apiContext.Context(), providerURL.String(), namespace, name, ids)
		if err != nil {
			return fmt.Errorf("failed to resolve auth groups: %w", err)
		}

		return apiContext.Write(types.GroupListResponse{
			Items:  trimGroupsForUser(apiContext.User, groups),
			Source: types.GroupSourceCache,
		})
	}

	limit, cursor := parseGroupListParams(query)
	result, err := apiContext.GatewayClient.ListAuthGroups(
		apiContext.Context(),
		providerURL.String(),
		namespace,
		name,
		client.ListAuthGroupsOptions{
			NameFilter: query.Get("name"),
			Limit:      limit,
			Cursor:     cursor,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to list auth groups: %w", err)
	}

	slog.Debug("Listed auth provider groups",
		"providerNamespace", namespace, "providerName", name,
		"groups", len(result.Groups), "hasMore", result.NextCursor != "",
		"source", result.Source, "degraded", result.Degraded, "reset", result.Reset)

	return apiContext.Write(types.GroupListResponse{
		Items:      trimGroupsForUser(apiContext.User, result.Groups),
		NextCursor: result.NextCursor,
		Source:     result.Source,
		Degraded:   result.Degraded,
		Reset:      result.Reset,
	})
}

func parseGroupListParams(query url.Values) (limit int, cursor string) {
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit <= 0 {
		limit = client.DefaultGroupPageSize
	}

	return min(limit, client.MaxGroupPageSize), query.Get("cursor")
}

// splitGroupIDs parses the comma-separated ids parameter, dropping blanks and duplicates.
// IDs are returned in the order they first appear in raw.
func splitGroupIDs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) > maxGroupIDsPerRequest {
		return nil, fmt.Errorf("too many group ids: %d, limit is %d", len(parts), maxGroupIDsPerRequest)
	}

	seen := make(map[string]struct{}, len(parts))
	ids := make([]string, 0, len(parts))

	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return ids, nil
}

// trimGroupsForUser strips everything but the ID and name for users who should not see which auth
// provider a group belongs to.
func trimGroupsForUser(u user.Info, groups []types.Group) []types.Group {
	if !userIsBasicOrPower(u) {
		return groups
	}

	trimmed := make([]types.Group, 0, len(groups))
	for _, group := range groups {
		trimmed = append(trimmed, types.Group{
			ID:   group.ID,
			Name: group.Name,
		})
	}

	return trimmed
}

// trimUser returns the only fields of a user that Basic and Power users may see: its ID, display name, and status.
// The status lets them tell which users are disabled, such as when choosing whom to share with.
func trimUser(u types.User) types2.User {
	return types2.User{
		ID:          fmt.Sprint(u.ID),
		DisplayName: u.DisplayName,
		Status:      u.Status(),
	}
}

func userIsBasicOrPower(u user.Info) bool {
	for _, group := range u.GetGroups() {
		switch group {
		case types2.GroupPowerUserPlus, types2.GroupAuditor, types2.GroupAdmin, types2.GroupOwner:
			return false
		}
	}
	return true
}
