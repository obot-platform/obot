package client

import (
	"context"
	"errors"
	"fmt"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
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

// GetGroupRoleAssignment returns a specific group role assignment by ID.
func (c *Client) GetGroupRoleAssignment(ctx context.Context, id string) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&assignment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("group role assignment %s not found", id)
		}
		return nil, fmt.Errorf("failed to get group role assignment: %w", err)
	}
	return &assignment, nil
}

// GetGroupRoleAssignmentByGroupName returns assignment for a specific group name.
// Returns nil if no assignment exists for this group (not an error).
func (c *Client) GetGroupRoleAssignmentByGroupName(ctx context.Context, groupName string) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment
	if err := c.db.WithContext(ctx).Where("group_name = ?", groupName).First(&assignment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No assignment for this group
		}
		return nil, fmt.Errorf("failed to get group role assignment: %w", err)
	}
	return &assignment, nil
}

// CreateGroupRoleAssignment creates a new group role assignment with a generated ID.
func (c *Client) CreateGroupRoleAssignment(ctx context.Context, groupName string, role types2.Role, description string) (*types.GroupRoleAssignment, error) {
	assignment := &types.GroupRoleAssignment{
		ID:          c.getGroupRoleAssignmentID(groupName),
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
func (c *Client) UpdateGroupRoleAssignment(ctx context.Context, id string, role types2.Role, description string) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&assignment).Error; err != nil {
			return err
		}

		assignment.Role = role
		assignment.Description = description

		return tx.Save(&assignment).Error
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("group role assignment %s not found", id)
		}
		return nil, fmt.Errorf("failed to update group role assignment: %w", err)
	}

	return &assignment, nil
}

// DeleteGroupRoleAssignment deletes a group role assignment by ID.
func (c *Client) DeleteGroupRoleAssignment(ctx context.Context, id string) error {
	result := c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.GroupRoleAssignment{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete group role assignment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("group role assignment %s not found", id)
	}

	return nil
}

func (c *Client) getGroupRoleAssignmentID(groupName string) string {
	return system.GroupRoleAssignmentPrefix + groupName
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
