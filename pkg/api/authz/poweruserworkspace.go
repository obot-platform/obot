package authz

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// PowerUserWorkspaceAuthorizerFunc checks if a user can access a PowerUserWorkspace
type PowerUserWorkspaceAuthorizerFunc func(req Request) (bool, *v1.PowerUserWorkspace, error)

// NewPowerUserWorkspaceAuthorizer creates an authorizer for PowerUserWorkspace resources
func NewPowerUserWorkspaceAuthorizer(req Request) PowerUserWorkspaceAuthorizerFunc {
	return func(req Request) (bool, *v1.PowerUserWorkspace, error) {
		// Admins can access any workspace
		if req.UserIsAdmin() {
			return true, nil, nil
		}

		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, req.Name); err != nil {
			return false, nil, err
		}

		// Users can only access their own workspace
		userID := req.User.GetUID()
		if workspace.Spec.UserID == userID {
			return true, &workspace, nil
		}

		return false, nil, fmt.Errorf("access denied to workspace %s", req.Name)
	}
}

// CanUserCreateInWorkspace checks if a user can create resources in a workspace
func CanUserCreateInWorkspace(req Request, workspaceID string) (bool, error) {
	// Admins can create resources anywhere
	if req.UserIsAdmin() {
		return true, nil
	}

	// Get the workspace
	var workspace v1.PowerUserWorkspace
	if err := req.Get(&workspace, workspaceID); err != nil {
		return false, fmt.Errorf("workspace %s not found: %w", workspaceID, err)
	}

	// Check if user owns the workspace
	if workspace.Spec.UserID != req.User.GetUID() {
		return false, fmt.Errorf("access denied to workspace %s", workspaceID)
	}

	// Check if user has required role for the operation
	user, err := req.GatewayClient.UserByID(req.Context(), req.User.GetUID())
	if err != nil {
		return false, fmt.Errorf("failed to get user: %w", err)
	}

	// User must have elevated role to use workspace features
	if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus && user.Role != types.RolePowerUser {
		return false, fmt.Errorf("insufficient privileges for workspace operations")
	}

	return true, nil
}

// CanUserCreateMultiUserMCPServer checks if user can create multi-user MCP servers
func CanUserCreateMultiUserMCPServer(req Request, workspaceID string) (bool, error) {
	// Admins can create multi-user servers anywhere
	if req.UserIsAdmin() {
		return true, nil
	}

	// For workspace operations, check workspace access first
	if workspaceID != "" {
		canCreate, err := CanUserCreateInWorkspace(req, workspaceID)
		if err != nil || !canCreate {
			return false, err
		}

		// Only PowerUserPlus and Admins can create multi-user servers
		user, err := req.GatewayClient.UserByID(req.Context(), req.User.GetUID())
		if err != nil {
			return false, fmt.Errorf("failed to get user: %w", err)
		}

		if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus {
			return false, fmt.Errorf("insufficient privileges for multi-user MCP server creation")
		}

		return true, nil
	}

	// For global operations (outside workspace), only admins
	return false, fmt.Errorf("non-admin users cannot create global multi-user MCP servers")
}

// CanUserManageAccessControlRules checks if user can manage Access Control Rules
func CanUserManageAccessControlRules(req Request, workspaceID string) (bool, error) {
	// Admins can manage ACRs anywhere
	if req.UserIsAdmin() {
		return true, nil
	}

	// For workspace operations, check workspace access first
	if workspaceID != "" {
		canCreate, err := CanUserCreateInWorkspace(req, workspaceID)
		if err != nil || !canCreate {
			return false, err
		}

		// Only PowerUserPlus and Admins can manage Access Control Rules
		user, err := req.GatewayClient.UserByID(req.Context(), req.User.GetUID())
		if err != nil {
			return false, fmt.Errorf("failed to get user: %w", err)
		}

		if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus {
			return false, fmt.Errorf("insufficient privileges for Access Control Rule management")
		}

		return true, nil
	}

	// For global operations (outside workspace), only admins
	return false, fmt.Errorf("non-admin users cannot manage global Access Control Rules")
}