package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// getGroupRoleAssignments returns all group role assignments.
func (s *Server) getGroupRoleAssignments(apiContext api.Context) error {
	assignments, err := apiContext.GatewayClient.ListGroupRoleAssignments(apiContext.Context())
	if err != nil {
		return fmt.Errorf("failed to get group role assignments: %v", err)
	}

	items := make([]types2.GroupRoleAssignment, len(assignments))
	for i, assignment := range assignments {
		items[i] = convertGroupRoleAssignment(&assignment)
	}

	return apiContext.Write(types2.GroupRoleAssignmentList{
		Items: items,
	})
}

// getGroupRoleAssignment returns a specific group role assignment.
func (s *Server) getGroupRoleAssignment(apiContext api.Context) error {
	groupName := apiContext.PathValue("groupName")
	if groupName == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "groupName path parameter is required")
	}

	assignment, err := apiContext.GatewayClient.GetGroupRoleAssignment(apiContext.Context(), groupName)
	if err != nil {
		if errors.Is(err, client.ErrGroupRoleAssignmentNotFound) {
			return types2.NewErrNotFound("group role assignment %s not found", groupName)
		}
		return fmt.Errorf("failed to get group role assignment: %v", err)
	}

	return apiContext.Write(convertGroupRoleAssignment(assignment))
}

// createGroupRoleAssignment creates a new group role assignment.
func (s *Server) createGroupRoleAssignment(apiContext api.Context) error {
	var req types2.GroupRoleAssignment
	if err := apiContext.Read(&req); err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid request body: %s", err.Error()))
	}

	// Validation
	if req.GroupName == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "group name is required")
	}
	if req.Role == types2.RoleUnknown {
		return types2.NewErrHTTP(http.StatusBadRequest, "role is required")
	}

	// Only allow assigning specific roles (not combined bitmasks)
	validRoles := []types2.Role{
		types2.RoleAdmin,
		types2.RolePowerUserPlus,
		types2.RolePowerUser,
	}
	if !slices.Contains(validRoles, req.Role) {
		return types2.NewErrHTTP(http.StatusBadRequest,
			"invalid role: must be one of Admin, PowerUserPlus, PowerUser")
	}

	created, err := apiContext.GatewayClient.CreateGroupRoleAssignment(
		apiContext.Context(),
		req.GroupName,
		req.Role,
		req.Description,
	)
	if err != nil {
		// Check for unique constraint violation
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return types2.NewErrHTTP(http.StatusConflict,
				fmt.Sprintf("group role assignment for group %q already exists", req.GroupName))
		}
		return fmt.Errorf("failed to create group role assignment: %v", err)
	}

	// Trigger reconciliation for all users in this group
	if err := s.triggerReconciliationForGroup(apiContext.Context(), apiContext, req.GroupName); err != nil {
		pkgLog.Warnf("failed to trigger reconciliation for group %s: %v", req.GroupName, err)
		// Don't fail the request - assignment was created successfully
	}

	return apiContext.Write(convertGroupRoleAssignment(created))
}

// updateGroupRoleAssignment updates an existing group role assignment.
func (s *Server) updateGroupRoleAssignment(apiContext api.Context) error {
	groupName := apiContext.PathValue("groupName")
	if groupName == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "groupName path parameter is required")
	}

	var req types2.GroupRoleAssignment
	if err := apiContext.Read(&req); err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, "invalid request body")
	}

	// Validation
	if req.Role == types2.RoleUnknown {
		return types2.NewErrHTTP(http.StatusBadRequest, "role is required")
	}

	validRoles := []types2.Role{
		types2.RoleAdmin,
		types2.RolePowerUserPlus,
		types2.RolePowerUser,
	}
	if !slices.Contains(validRoles, req.Role) {
		return types2.NewErrHTTP(http.StatusBadRequest,
			"invalid role: must be one of Admin, PowerUserPlus, PowerUser")
	}

	updated, err := apiContext.GatewayClient.UpdateGroupRoleAssignment(
		apiContext.Context(), groupName, req.Role, req.Description)
	if err != nil {
		if errors.Is(err, client.ErrGroupRoleAssignmentNotFound) {
			return types2.NewErrNotFound("group role assignment %s not found", groupName)
		}
		return fmt.Errorf("failed to update group role assignment: %v", err)
	}

	// Trigger reconciliation for all users in this group
	if err := s.triggerReconciliationForGroup(apiContext.Context(), apiContext, groupName); err != nil {
		pkgLog.Warnf("failed to trigger reconciliation for group %s: %v", groupName, err)
		// Don't fail the request - assignment was updated successfully
	}

	return apiContext.Write(convertGroupRoleAssignment(updated))
}

// deleteGroupRoleAssignment deletes a group role assignment.
func (s *Server) deleteGroupRoleAssignment(apiContext api.Context) error {
	groupName := apiContext.PathValue("groupName")
	if groupName == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "groupName path parameter is required")
	}

	if err := apiContext.GatewayClient.DeleteGroupRoleAssignment(apiContext.Context(), groupName); err != nil {
		if errors.Is(err, client.ErrGroupRoleAssignmentNotFound) {
			return types2.NewErrNotFound("group role assignment %s not found", groupName)
		}
		return fmt.Errorf("failed to delete group role assignment: %v", err)
	}

	// Trigger reconciliation for all users in this group
	if err := s.triggerReconciliationForGroup(apiContext.Context(), apiContext, groupName); err != nil {
		pkgLog.Warnf("failed to trigger reconciliation for group %s: %v", groupName, err)
		// Don't fail the request - assignment was deleted successfully
	}

	return apiContext.Write(types2.GroupRoleAssignment{})
}

// triggerReconciliationForGroup creates UserRoleChange events for all users in the given group
// to trigger workspace reconciliation based on their current effective role.
func (s *Server) triggerReconciliationForGroup(ctx context.Context, apiContext api.Context, groupName string) error {
	// Get all users in this group
	users, err := apiContext.GatewayClient.GetUsersInGroup(ctx, groupName)
	if err != nil {
		return fmt.Errorf("failed to get users in group %s: %w", groupName, err)
	}

	// Create UserRoleChange event for each user to trigger reconciliation
	for _, user := range users {
		if err := apiContext.Create(&v1.UserRoleChange{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.UserRoleChangePrefix,
				Namespace:    apiContext.Namespace(),
			},
			Spec: v1.UserRoleChangeSpec{
				UserID: user.ID,
			},
		}); err != nil {
			pkgLog.Errorf("failed to create user role change event for user %d: %v", user.ID, err)
			// Continue processing other users even if one fails
		}
	}

	return nil
}

// convertGroupRoleAssignment converts database model to API type.
func convertGroupRoleAssignment(assignment *types.GroupRoleAssignment) types2.GroupRoleAssignment {
	return types2.GroupRoleAssignment{
		GroupName:   assignment.GroupName,
		Role:        assignment.Role,
		Description: assignment.Description,
	}
}
