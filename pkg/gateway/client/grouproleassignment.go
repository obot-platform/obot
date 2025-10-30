package client

import (
	"context"
	"errors"
	"fmt"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/exp/maps"
	"gorm.io/gorm"
)

// GetGroupRoleAssignments returns all group role assignments from the database.
func (c *Client) GetGroupRoleAssignments(ctx context.Context) ([]types.GroupRoleAssignment, error) {
	var assignments []types.GroupRoleAssignment
	if err := c.db.WithContext(ctx).Order("group_name").Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("failed to get group role assignments: %w", err)
	}
	return assignments, nil
}

// GetGroupRoleAssignment returns a specific group role assignment by group name.
func (c *Client) GetGroupRoleAssignment(ctx context.Context, groupName string) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment
	if err := c.db.WithContext(ctx).Where("group_name = ?", groupName).First(&assignment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("group role assignment %s not found", groupName)
		}
		return nil, fmt.Errorf("failed to get group role assignment: %w", err)
	}
	return &assignment, nil
}

// CreateGroupRoleAssignment creates a new group role assignment.
func (c *Client) CreateGroupRoleAssignment(ctx context.Context, groupName string, role types2.Role, description string) (*types.GroupRoleAssignment, error) {
	assignment := &types.GroupRoleAssignment{
		GroupName:   groupName,
		Role:        role,
		Description: description,
	}

	if err := c.db.WithContext(ctx).Create(assignment).Error; err != nil {
		return nil, fmt.Errorf("failed to create group role assignment: %w", err)
	}

	return assignment, nil
}

// UpdateGroupRoleAssignment updates an existing group role assignment.
func (c *Client) UpdateGroupRoleAssignment(ctx context.Context, groupName string, role types2.Role, description string) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_name = ?", groupName).First(&assignment).Error; err != nil {
			return err
		}

		assignment.Role = role
		assignment.Description = description

		return tx.Save(&assignment).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("group role assignment %s not found", groupName)
		}
		return nil, fmt.Errorf("failed to update group role assignment: %w", err)
	}

	return &assignment, nil
}

// DeleteGroupRoleAssignment deletes a group role assignment by group name.
func (c *Client) DeleteGroupRoleAssignment(ctx context.Context, groupName string) error {
	result := c.db.WithContext(ctx).Where("group_name = ?", groupName).Delete(&types.GroupRoleAssignment{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete group role assignment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("group role assignment %s not found", groupName)
	}

	return nil
}

// GetGroupRoleAssignmentsForGroups retrieves all role assignments for the given group names.
// This is used during role resolution to find all roles assigned to a user's groups.
func (c *Client) GetGroupRoleAssignmentsForGroups(ctx context.Context, groupNames []string) ([]types.GroupRoleAssignment, error) {
	if len(groupNames) == 0 {
		return nil, nil
	}

	var assignments []types.GroupRoleAssignment
	if err := c.db.WithContext(ctx).Where("group_name IN ?", groupNames).Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("failed to get group role assignments: %w", err)
	}

	return assignments, nil
}

// ResolveUserEffectiveRole computes the effective role for a user by combining:
// 1. Individual role from users table
// 2. Group-based roles from GroupRoleAssignments
// Returns the merged role with the highest permissions.
func (c *Client) ResolveUserEffectiveRole(ctx context.Context, user *types.User, authGroupIDs []string) (types2.Role, error) {
	// Start with user's individual role
	effectiveRole := user.Role

	// If no auth provider groups, return individual role
	if len(authGroupIDs) == 0 {
		return effectiveRole, nil
	}

	// Query database for group role assignments matching user's groups
	// We need to extract group names from the auth group IDs
	// Auth group IDs look like: "github:org/team", "entra:group-uuid", etc.
	// For GroupRoleAssignments, we'll match on the full group ID as the GroupName
	assignments, err := c.GetGroupRoleAssignmentsForGroups(ctx, authGroupIDs)
	if err != nil {
		// Don't fail role resolution if query fails - fall back to individual role
		// Note: We're returning nil error here because we don't want to fail authentication
		return effectiveRole, nil
	}

	// Merge all group roles using bitwise OR
	for _, assignment := range assignments {
		effectiveRole |= assignment.Role
	}

	return effectiveRole, nil
}

// ResolveUserEffectiveRolesBulk computes effective roles for multiple users efficiently.
// It performs a single database query to fetch all group role assignments for all users' groups.
// Returns a map of userID to their effective role.
func (c *Client) ResolveUserEffectiveRolesBulk(ctx context.Context, users []types.User, userGroupMemberships map[uint][]string) (map[uint]types2.Role, error) {
	effectiveRoles := make(map[uint]types2.Role, len(users))

	// Collect all unique group IDs across all users
	uniqueGroupIDs := make(map[string]struct{})
	for _, groupIDs := range userGroupMemberships {
		for _, groupID := range groupIDs {
			uniqueGroupIDs[groupID] = struct{}{}
		}
	}

	// If no groups at all, just return individual roles
	if len(uniqueGroupIDs) == 0 {
		for _, user := range users {
			effectiveRoles[user.ID] = user.Role
		}
		return effectiveRoles, nil
	}

	// Fetch all group role assignments in one query
	assignments, err := c.GetGroupRoleAssignmentsForGroups(ctx, maps.Keys(uniqueGroupIDs))
	if err != nil {
		// Don't fail - fall back to individual roles
		for _, user := range users {
			effectiveRoles[user.ID] = user.Role
		}
		return effectiveRoles, nil
	}

	// Build group -> role map for fast lookup
	groupRoles := make(map[string]types2.Role)
	for _, assignment := range assignments {
		groupRoles[assignment.GroupName] = assignment.Role
	}

	// Compute effective role for each user
	for _, user := range users {
		effectiveRole := user.Role

		// If user has groups, merge their roles
		if userGroups, ok := userGroupMemberships[user.ID]; ok {
			for _, groupID := range userGroups {
				if groupRole, exists := groupRoles[groupID]; exists {
					effectiveRole |= groupRole
				}
			}
		}

		effectiveRoles[user.ID] = effectiveRole
	}

	return effectiveRoles, nil
}
