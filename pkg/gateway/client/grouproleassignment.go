package client

import (
	"context"
	"errors"
	"fmt"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/exp/maps"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ownerManagedGroupRoles = types2.RoleOwner | types2.RoleAuditor
)

var (
	// ErrGroupRoleAssignmentNotFound is returned when a group role assignment is not found.
	ErrGroupRoleAssignmentNotFound = errors.New("group role assignment not found")
	// ErrGroupRoleAssignmentOwnerRequired protects Owner and Auditor grants.
	ErrGroupRoleAssignmentOwnerRequired = errors.New("only owners can change owner or auditor group grants")
)

// ListGroupRoleAssignments returns all group role assignments from the database.
func (c *Client) ListGroupRoleAssignments(ctx context.Context) ([]types.GroupRoleAssignment, error) {
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
			return nil, fmt.Errorf("%w: %s", ErrGroupRoleAssignmentNotFound, groupName)
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

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockRoleGroups(tx, []string{groupName}, clause.LockingStrengthUpdate); err != nil {
			return err
		}
		if err := tx.Create(assignment).Error; err != nil {
			return err
		}
		return clearReadonlyAdminForPromotedGroup(tx, groupName, role)
	}); err != nil {
		return nil, err
	}

	return assignment, nil
}

// UpdateGroupRoleAssignment updates an existing group role assignment.
func (c *Client) UpdateGroupRoleAssignment(ctx context.Context, groupName string, role types2.Role, description string, isOwner bool) (*types.GroupRoleAssignment, error) {
	var assignment types.GroupRoleAssignment

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockRoleGroups(tx, []string{groupName}, clause.LockingStrengthUpdate); err != nil {
			return err
		}
		if err := tx.Where("group_name = ?", groupName).First(&assignment).Error; err != nil {
			return err
		}

		update := tx.Model(&assignment)
		// Match the requested privileged bits at write time so a concurrent Owner update
		// cannot be overwritten by an Admin changing a different grant.
		if !isOwner {
			update = update.Where("role & ? = ?", ownerManagedGroupRoles, role&ownerManagedGroupRoles)
		}
		result := update.Updates(map[string]any{"role": role, "description": description})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrGroupRoleAssignmentOwnerRequired
		}
		assignment.Role = role
		assignment.Description = description
		return clearReadonlyAdminForPromotedGroup(tx, groupName, role)
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrGroupRoleAssignmentNotFound, groupName)
		}
		return nil, fmt.Errorf("failed to update group role assignment: %w", err)
	}

	return &assignment, nil
}

func lockRoleGroups(tx *gorm.DB, groupIDs []string, strength string) error {
	return lockGroupRows(tx.Where("id IN ?", groupIDs), strength)
}

// Lock group rows in the same order as refreshes that need different strengths for different groups.
func lockGroupRows(groups *gorm.DB, strength string) error {
	order := "id"
	if groups.Name() == "postgres" {
		// Match Go's byte order for refreshes that acquire their locks in several batches.
		order = `id COLLATE "C"`
	}
	var ids []string
	return groups.Model(new(types.Group)).Clauses(clause.Locking{Strength: strength}).
		Order(order).Pluck("id", &ids).Error
}

func elevatedGroupMembers(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Table("group_memberships").
		Joins("JOIN group_role_assignments ON group_role_assignments.group_name = group_memberships.group_id").
		Where("group_role_assignments.role & ? != 0", types2.RoleAdmin|types2.RoleOwner)
}

func clearReadonlyAdminForPromotedGroup(tx *gorm.DB, groupName string, role types2.Role) error {
	if !role.HasRole(types2.RoleAdmin) {
		return nil
	}
	members := tx.Model(new(types.GroupMemberships)).Select("user_id").Where("group_id = ?", groupName)
	var ids []uint
	// Lock every affected user, even without a direct grant, to serialize against concurrent direct assignment.
	if err := tx.Model(new(types.User)).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN (?)", members).Order("id").Pluck("id", &ids).Error; err != nil {
		return err
	}
	return clearReadonlyAdminForElevatedUsers(tx, ids)
}

func clearReadonlyAdminForElevatedUsers(tx *gorm.DB, ids []uint) error {
	for batch := range slices.Chunk(ids, scimMemberBatchSize) {
		elevated := elevatedGroupMembers(tx).Select("group_memberships.user_id").
			Where("group_memberships.user_id IN ?", batch)
		if err := tx.Model(new(types.User)).Where("id IN (?) AND role & ? != 0", elevated, types2.RoleAdminReadonly).
			UpdateColumn("role", gorm.Expr("role & ?", ^types2.RoleAdminReadonly)).Error; err != nil {
			return fmt.Errorf("failed to remove Read-Only Admin from elevated users: %w", err)
		}
	}
	return nil
}

// Lock membership additions against role edits before writing the new edges.
func lockMembershipAddsForRoles(tx *gorm.DB, groupIDs []string, userIDs []uint) error {
	if err := lockRoleGroups(tx, groupIDs, clause.LockingStrengthShare); err != nil {
		return err
	}
	return lockUsersForRoleChanges(tx, userIDs)
}

func lockUsersForRoleChanges(tx *gorm.DB, userIDs []uint) error {
	for batch := range slices.Chunk(userIDs, scimMemberBatchSize) {
		var locked []uint
		if err := tx.Model(new(types.User)).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", batch).Order("id").Pluck("id", &locked).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteGroupRoleAssignment deletes a group role assignment by group name.
func (c *Client) DeleteGroupRoleAssignment(ctx context.Context, groupName string, isOwner bool) error {
	query := c.db.WithContext(ctx).Where("group_name = ?", groupName)
	if !isOwner {
		query = query.Where("role & ? = 0", ownerManagedGroupRoles)
	}
	result := query.Delete(&types.GroupRoleAssignment{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete group role assignment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := c.GetGroupRoleAssignment(ctx, groupName); err != nil {
			return err
		}
		return ErrGroupRoleAssignmentOwnerRequired
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
// Returns the highest base role plus orthogonal add-on roles (if present).
func (c *Client) ResolveUserEffectiveRole(ctx context.Context, user *types.User, authGroupIDs []string) (types2.Role, error) {
	// Start with user's individual role
	effectiveRole := user.Role

	if len(authGroupIDs) == 0 {
		return effectiveRole.WithoutRedundantReadonlyAdmin(), nil
	}

	// Query database for group role assignments matching user's groups
	// We need to extract group names from the auth group IDs
	// Auth group IDs look like: "github:org/team", "entra:group-uuid", etc.
	// For GroupRoleAssignments, we'll match on the full group ID as the GroupName
	assignments, err := c.GetGroupRoleAssignmentsForGroups(ctx, authGroupIDs)
	if err != nil {
		return effectiveRole, err
	}

	// Merge all group roles using bitwise OR
	for _, assignment := range assignments {
		effectiveRole |= assignment.Role
	}

	// Normalize to keep only the highest base role + add-on roles
	return normalizeToHighestRole(effectiveRole), nil
}

// normalizeToHighestRole takes a combined role bitmap and returns only the highest
// base role (Owner > Admin > PowerUserPlus > PowerUser > Basic) plus add-on bits if present.
func normalizeToHighestRole(combinedRole types2.Role) types2.Role {
	// Find the highest base role in descending order of privilege
	var highestRole types2.Role
	if combinedRole.HasRole(types2.RoleOwner) {
		highestRole = types2.RoleOwner
	} else if combinedRole.HasRole(types2.RoleAdmin) {
		highestRole = types2.RoleAdmin
	} else if combinedRole.HasRole(types2.RolePowerUserPlus) {
		highestRole = types2.RolePowerUserPlus
	} else if combinedRole.HasRole(types2.RolePowerUser) {
		highestRole = types2.RolePowerUser
	} else {
		highestRole = types2.RoleBasic
	}

	return (highestRole | combinedRole.AddOnRoles()).WithoutRedundantReadonlyAdmin()
}

// GetUsersInGroup returns all users who are members of the given group.
// This is used to find users affected by GroupRoleAssignment changes.
func (c *Client) GetUsersInGroup(ctx context.Context, groupName string) ([]types.User, error) {
	var users []types.User
	err := c.db.WithContext(ctx).
		Table("users").
		Joins("JOIN group_memberships ON group_memberships.user_id = users.id").
		Where("group_memberships.group_id = ?", groupName).
		Find(&users).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get users in group %s: %w", groupName, err)
	}

	return users, nil
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
			effectiveRoles[user.ID] = user.Role.WithoutRedundantReadonlyAdmin()
		}
		return effectiveRoles, nil
	}

	// Fetch all group role assignments in one query
	assignments, err := c.GetGroupRoleAssignmentsForGroups(ctx, maps.Keys(uniqueGroupIDs))
	if err != nil {
		// Don't fail - fall back to individual roles
		for _, user := range users {
			effectiveRoles[user.ID] = user.Role.WithoutRedundantReadonlyAdmin()
		}
		return effectiveRoles, nil
	}

	// Build group -> role map for fast lookup
	groupRoles := make(map[string]types2.Role, len(assignments))
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

		// Normalize to keep only the highest base role + add-on roles
		effectiveRoles[user.ID] = normalizeToHighestRole(effectiveRole)
	}

	return effectiveRoles, nil
}
