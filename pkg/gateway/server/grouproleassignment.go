package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// getGroupRoleAssignments returns all group role assignments.
func (s *Server) getGroupRoleAssignments(apiContext api.Context) error {
	assignments, err := apiContext.GatewayClient.GetGroupRoleAssignments(apiContext.Context())
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
	id := apiContext.PathValue("id")
	if id == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "id path parameter is required")
	}

	assignment, err := apiContext.GatewayClient.GetGroupRoleAssignment(apiContext.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return types2.NewErrNotFound("group role assignment %s not found", id)
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
		return types2.NewErrHTTP(http.StatusBadRequest, "groupName is required")
	}
	if req.Role == types2.RoleUnknown {
		return types2.NewErrHTTP(http.StatusBadRequest, "role is required")
	}

	// Only allow assigning specific roles (not combined bitmasks)
	validRoles := []types2.Role{
		types2.RoleBasic,
		types2.RoleOwner,
		types2.RoleAdmin,
		types2.RolePowerUserPlus,
		types2.RolePowerUser,
	}
	if !slices.Contains(validRoles, req.Role) {
		return types2.NewErrHTTP(http.StatusBadRequest,
			"invalid role: must be one of Basic, Owner, Admin, PowerUserPlus, PowerUser")
	}

	created, err := apiContext.GatewayClient.CreateGroupRoleAssignment(
		apiContext.Context(),
		req.GroupName,
		req.Role,
		req.Description,
	)
	if err != nil {
		// Check for unique constraint violation
		// TODO(g-linville): see if these can be replaces with proper errors.Is/As checks
		if strings.Contains(err.Error(), "UNIQUE constraint") ||
			strings.Contains(err.Error(), "duplicate key") ||
			strings.Contains(err.Error(), "unique constraint") {
			return types2.NewErrHTTP(http.StatusConflict,
				fmt.Sprintf("group role assignment for group %q already exists", req.GroupName))
		}
		return fmt.Errorf("failed to create group role assignment: %v", err)
	}

	return apiContext.Write(convertGroupRoleAssignment(created))
}

// updateGroupRoleAssignment updates an existing group role assignment.
func (s *Server) updateGroupRoleAssignment(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	if id == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "id path parameter is required")
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
		types2.RoleBasic,
		types2.RoleOwner,
		types2.RoleAdmin,
		types2.RoleAuditor,
		types2.RolePowerUserPlus,
		types2.RolePowerUser,
	}
	if !slices.Contains(validRoles, req.Role) {
		return types2.NewErrHTTP(http.StatusBadRequest,
			"invalid role: must be one of Basic, Owner, Admin, Auditor, PowerUserPlus, PowerUser")
	}

	updated, err := apiContext.GatewayClient.UpdateGroupRoleAssignment(
		apiContext.Context(), id, req.Role, req.Description)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return types2.NewErrNotFound("group role assignment %s not found", id)
		}
		return fmt.Errorf("failed to update group role assignment: %v", err)
	}

	return apiContext.Write(convertGroupRoleAssignment(updated))
}

// deleteGroupRoleAssignment deletes a group role assignment.
func (s *Server) deleteGroupRoleAssignment(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	if id == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "id path parameter is required")
	}

	if err := apiContext.GatewayClient.DeleteGroupRoleAssignment(apiContext.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return types2.NewErrNotFound("group role assignment %s not found", id)
		}
		return fmt.Errorf("failed to delete group role assignment: %v", err)
	}

	return apiContext.Write(types2.GroupRoleAssignment{})
}

// convertGroupRoleAssignment converts database model to API type.
func convertGroupRoleAssignment(assignment *types.GroupRoleAssignment) types2.GroupRoleAssignment {
	return types2.GroupRoleAssignment{
		Metadata: types2.Metadata{
			ID:      assignment.ID,
			Created: *types2.NewTime(assignment.CreatedAt),
		},
		GroupName:   assignment.GroupName,
		Role:        assignment.Role,
		Description: assignment.Description,
	}
}
