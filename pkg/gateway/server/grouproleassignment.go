package server

import (
	"errors"
	"fmt"
	"net/http"

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
		return types2.NewErrBadRequest("groupName is required")
	}
	if req.Role == types2.RoleUnknown {
		return types2.NewErrBadRequest("role is required")
	}

	// Validate that the requester is authorized to assign this role
	if err := s.validateRoleForUser(apiContext, req.Role); err != nil {
		return err
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
	if err := s.triggerReconciliationForGroup(apiContext, req.GroupName); err != nil {
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
		return types2.NewErrBadRequest("failed to decode request: %v", err)
	}
	if req.Role == types2.RoleUnknown {
		return types2.NewErrBadRequest("role is required")
	}

	// Validate that the requester is authorized to assign this role
	if err := s.validateRoleForUser(apiContext, req.Role); err != nil {
		return err
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
	if err := s.triggerReconciliationForGroup(apiContext, groupName); err != nil {
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
	if err := s.triggerReconciliationForGroup(apiContext, groupName); err != nil {
		pkgLog.Warnf("failed to trigger reconciliation for group %s: %v", groupName, err)
		// Don't fail the request - assignment was deleted successfully
	}

	return apiContext.Write(types2.GroupRoleAssignment{})
}

// triggerReconciliationForGroup creates a GroupRoleChange event for the given group
// to trigger workspace reconciliation for all users in the group based on their current effective role.
func (s *Server) triggerReconciliationForGroup(apiContext api.Context, groupName string) error {
	// Create a single GroupRoleChange event that will trigger reconciliation for all users in the group
	if err := apiContext.Create(&v1.GroupRoleChange{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.GroupRoleChangePrefix,
			Namespace:    apiContext.Namespace(),
		},
		Spec: v1.GroupRoleChangeSpec{
			GroupName: groupName,
		},
	}); err != nil {
		return fmt.Errorf("failed to create group role change event for group %s: %w", groupName, err)
	}

	return nil
}

// validateRoleForUser checks if the requester is authorized to assign the given role
func (s *Server) validateRoleForUser(apiContext api.Context, role types2.Role) error {
	// Extract base role and auditor flag
	baseRole := role.ExtractBaseRole()
	hasAuditor := role.HasAuditorRole()

	// If role includes Owner, only Owners can assign it
	if baseRole == types2.RoleOwner {
		if !apiContext.UserIsOwner() {
			return types2.NewErrHTTP(http.StatusForbidden, "only owners can assign the owner role to groups")
		}
	}

	// If role includes Auditor (even by itself), only Owners can assign it
	if hasAuditor {
		if !apiContext.UserIsOwner() {
			return types2.NewErrHTTP(http.StatusForbidden, "only owners can assign the auditor role to groups")
		}
	}

	// Validate base role is one of the allowed values (or zero if only Auditor)
	if baseRole != 0 {
		validBaseRoles := []types2.Role{
			types2.RoleOwner,
			types2.RoleAdmin,
			types2.RolePowerUserPlus,
			types2.RolePowerUser,
		}

		isValid := false
		for _, validRole := range validBaseRoles {
			if baseRole == validRole {
				isValid = true
				break
			}
		}

		if !isValid {
			return types2.NewErrBadRequest(
				"base role must be one of: Owner (%d), Admin (%d), PowerUserPlus (%d), or PowerUser (%d)",
				types2.RoleOwner, types2.RoleAdmin, types2.RolePowerUserPlus, types2.RolePowerUser,
			)
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
