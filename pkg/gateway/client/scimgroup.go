package client

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// scimMemberBatchSize bounds the number of parameters in a single membership query.
	scimMemberBatchSize = 500
)

// SCIMGroupInput holds the writable attributes of a SCIM group, as a create or a full replacement sends them.
type SCIMGroupInput struct {
	DisplayName string
	// MemberIDs are the SCIM user IDs of the complete member set.
	MemberIDs []string
}

// SCIMGroup is a SCIM group as the SCIM endpoint serves it.
type SCIMGroup struct {
	ID string
	// GroupID is the ID of the Obot group the SCIM group is bound to.
	GroupID     string
	DisplayName string
	// Members is nil unless members were requested.
	Members   []SCIMGroupMember
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SCIMGroupMember is a bound user in a SCIM group.
type SCIMGroupMember struct {
	ID       string
	UserName string
}

// SCIMGroupFilter selects SCIM groups. Empty fields match everything.
type SCIMGroupFilter struct {
	ID          string
	DisplayName string
}

type scimGroupRow struct {
	types.SCIMGroupBinding
	Name string
}

// ListSCIMGroups returns the connection's bound groups that match filter, oldest first, and the total number of
// matches. Groups that SCIM has not bound are never returned.
func (c *Client) ListSCIMGroups(ctx context.Context, connectionID string, filter SCIMGroupFilter, page SCIMPage, includeMembers bool) ([]SCIMGroup, int64, error) {
	var (
		groups []SCIMGroup
		total  int64
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := scimGroupQuery(tx, connectionID)
		if filter.ID != "" {
			query = query.Where("scim_group_bindings.id = ?", filter.ID)
		}
		if filter.DisplayName != "" {
			query = query.Where("scim_group_bindings.normalized_display_name = ?", types.NormalizeSCIMName(filter.DisplayName))
		}

		if err := query.Count(&total).Error; err != nil {
			return fmt.Errorf("failed to count SCIM groups: %w", err)
		}
		if page.Limit <= 0 || int64(page.Offset) >= total {
			return nil
		}

		var rows []scimGroupRow
		if err := query.Select("scim_group_bindings.*, groups.name AS name").
			Order("scim_group_bindings.created_at, scim_group_bindings.id").
			Offset(page.Offset).
			Limit(page.Limit).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("failed to list SCIM groups: %w", err)
		}

		var members map[string][]SCIMGroupMember
		if includeMembers {
			groupIDs := make([]string, 0, len(rows))
			for _, row := range rows {
				groupIDs = append(groupIDs, row.GroupID)
			}

			var err error
			if members, err = c.scimGroupMembersTx(ctx, tx, connectionID, groupIDs); err != nil {
				return err
			}
		}

		groups = make([]SCIMGroup, 0, len(rows))
		for _, row := range rows {
			groups = append(groups, scimGroupFromRow(row, members, includeMembers))
		}
		return nil
	}); err != nil {
		return nil, 0, err
	}

	return groups, total, nil
}

// GetSCIMGroup returns the connection's bound group with the given SCIM ID.
func (c *Client) GetSCIMGroup(ctx context.Context, connectionID, id string, includeMembers bool) (*SCIMGroup, error) {
	var group *SCIMGroup
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		group, err = c.scimGroupTx(ctx, tx, connectionID, id, includeMembers, false)
		return err
	}); err != nil {
		return nil, err
	}
	return group, nil
}

// CreateSCIMGroup binds a pushed group to the one unbound group of the connection's auth provider with the same
// normalized name, keeping that group's ID, or creates a group when there is none. The pushed members replace the
// group's memberships in the same transaction: cached members that the identity provider did not push are removed,
// and members that remain are not touched.
func (c *Client) CreateSCIMGroup(ctx context.Context, conn *types.SCIMConnection, input SCIMGroupInput) (*SCIMGroup, error) {
	a, err := connectionAdapter(conn)
	if err != nil {
		return nil, err
	}

	normalized := types.NormalizeSCIMName(input.DisplayName)
	if normalized == "" {
		return nil, &SCIMInvalidValueError{
			Message: "displayName is required",
		}
	}

	var (
		group   *SCIMGroup
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}
		// A bound group with the same name is a duplicate in the identity provider, which binding must not resolve.
		if err := checkSCIMGroupNameTx(tx, conn.ID, normalized, input.DisplayName, ""); err != nil {
			return err
		}

		members, err := resolveSCIMMembersTx(tx, conn.ID, input.MemberIDs)
		if err != nil {
			return err
		}

		candidates, err := unboundGroupsNamedTx(tx, conn, normalized)
		if err != nil {
			return err
		}

		binding := &types.SCIMGroupBinding{
			ID:                    uuid.NewV4().String(),
			ConnectionID:          conn.ID,
			NormalizedDisplayName: normalized,
			Revision:              1,
		}
		switch len(candidates) {
		case 0:
			binding.GroupID = a.NewGroupID(conn.GroupIDPrefix, binding.ID)
			binding.Origin = types.SCIMGroupBindingOriginCreated
			if err := tx.Create(&types.Group{
				ID:                    binding.GroupID,
				AuthProviderName:      conn.AuthProviderName,
				AuthProviderNamespace: conn.AuthProviderNamespace,
				Name:                  input.DisplayName,
			}).Error; err != nil {
				return fmt.Errorf("failed to create group: %w", err)
			}
		case 1:
			binding.GroupID = candidates[0].ID
			binding.Origin = types.SCIMGroupBindingOriginExisting
			if candidates[0].Name != input.DisplayName {
				if err := renameGroupTx(tx, binding.GroupID, input.DisplayName); err != nil {
					return err
				}
			}
		default:
			return &SCIMConflictError{
				Message: fmt.Sprintf("%d existing groups are named %q; remove the references to all but one of them in Obot", len(candidates), input.DisplayName),
			}
		}

		if err := tx.Create(binding).Error; err != nil {
			return fmt.Errorf("failed to create SCIM group binding: %w", err)
		}

		changes, err := replaceGroupMembershipsTx(tx, binding.GroupID, members)
		if err != nil {
			return err
		}
		if err := recordMembershipReconcileEventsTx(tx, changes); err != nil {
			return err
		}
		changed = len(changes) > 0

		group, err = c.scimGroupTx(ctx, tx, conn.ID, binding.ID, true, false)
		return err
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	return group, nil
}

// UpdateSCIMGroup changes a bound group's display name and members. The replacement is computed by mutate from the
// group's current state, with its members, inside the transaction that writes it. A replacement identical to the
// current state changes nothing and emits nothing.
func (c *Client) UpdateSCIMGroup(ctx context.Context, conn *types.SCIMConnection, id string, mutate func(current SCIMGroup) (SCIMGroupInput, error)) (*SCIMGroup, error) {
	var (
		group   *SCIMGroup
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		current, err := c.scimGroupTx(ctx, tx, conn.ID, id, true, true)
		if err != nil {
			return err
		}

		input, err := mutate(*current)
		if err != nil {
			return err
		}

		normalized := types.NormalizeSCIMName(input.DisplayName)
		if normalized == "" {
			return &SCIMInvalidValueError{
				Message: "displayName is required",
			}
		}

		members, err := resolveSCIMMembersTx(tx, conn.ID, input.MemberIDs)
		if err != nil {
			return err
		}

		renamed := input.DisplayName != current.DisplayName
		if renamed {
			if err := checkSCIMGroupNameTx(tx, conn.ID, normalized, input.DisplayName, id); err != nil {
				return err
			}
			if err := renameGroupTx(tx, current.GroupID, input.DisplayName); err != nil {
				return err
			}
		}

		changes, err := replaceGroupMembershipsTx(tx, current.GroupID, members)
		if err != nil {
			return err
		}
		if err := recordMembershipReconcileEventsTx(tx, changes); err != nil {
			return err
		}
		changed = len(changes) > 0

		if renamed || changed {
			if err := tx.Model(new(types.SCIMGroupBinding)).Where("id = ?", id).UpdateColumns(map[string]any{
				"normalized_display_name": normalized,
				"revision":                gorm.Expr("revision + 1"),
				"updated_at":              time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("failed to update SCIM group binding: %w", err)
			}
		}

		group, err = c.scimGroupTx(ctx, tx, conn.ID, id, true, false)
		return err
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	return group, nil
}

// DeleteSCIMGroup retires a group's binding and removes its memberships. The group and every reference to it remain,
// and the group becomes unbound again: pushing it later under the same name binds it under a new SCIM ID and restores
// its members.
func (c *Client) DeleteSCIMGroup(ctx context.Context, conn *types.SCIMConnection, id string) error {
	var changed bool
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		var bindings []types.SCIMGroupBinding
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND connection_id = ? AND retired_at IS NULL", id, conn.ID).
			Limit(1).
			Find(&bindings).Error; err != nil {
			return fmt.Errorf("failed to get SCIM group %s: %w", id, err)
		}
		if len(bindings) == 0 {
			return &SCIMNotFoundError{
				ResourceType: types.SCIMResourceTypeGroup,
				ID:           id,
			}
		}
		binding := &bindings[0]

		now := time.Now()
		if err := tx.Model(binding).UpdateColumns(map[string]any{
			"retired_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("failed to retire SCIM group binding: %w", err)
		}

		changes, err := replaceGroupMembershipsTx(tx, binding.GroupID, nil)
		if err != nil {
			return err
		}
		if err := recordMembershipReconcileEventsTx(tx, changes); err != nil {
			return err
		}
		changed = len(changes) > 0
		return nil
	}); err != nil {
		return err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}
	return nil
}

func (c *Client) scimGroupTx(ctx context.Context, tx *gorm.DB, connectionID, id string, includeMembers, forUpdate bool) (*SCIMGroup, error) {
	query := scimGroupQuery(tx, connectionID).Where("scim_group_bindings.id = ?", id)
	if forUpdate {
		query = query.Clauses(clause.Locking{
			Strength: "UPDATE",
			Table: clause.Table{
				Name: "scim_group_bindings",
			},
		})
	}

	var rows []scimGroupRow
	if err := query.Select("scim_group_bindings.*, groups.name AS name").Limit(1).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get SCIM group %s: %w", id, err)
	}
	if len(rows) == 0 {
		return nil, &SCIMNotFoundError{
			ResourceType: types.SCIMResourceTypeGroup,
			ID:           id,
		}
	}

	var members map[string][]SCIMGroupMember
	if includeMembers {
		var err error
		if members, err = c.scimGroupMembersTx(ctx, tx, connectionID, []string{rows[0].GroupID}); err != nil {
			return nil, err
		}
	}

	group := scimGroupFromRow(rows[0], members, includeMembers)
	return &group, nil
}

// scimGroupMembersTx returns the bound users of the connection in each group.
func (c *Client) scimGroupMembersTx(ctx context.Context, tx *gorm.DB, connectionID string, groupIDs []string) (map[string][]SCIMGroupMember, error) {
	members := make(map[string][]SCIMGroupMember, len(groupIDs))
	if len(groupIDs) == 0 {
		return members, nil
	}

	type memberRow struct {
		types.SCIMUserBinding
		GroupID string
	}
	var rows []memberRow
	for batch := range slices.Chunk(groupIDs, scimMemberBatchSize) {
		var batchRows []memberRow
		if err := tx.Table("group_memberships").
			Select("scim_user_bindings.*, group_memberships.group_id AS group_id").
			Joins("JOIN scim_user_bindings ON scim_user_bindings.user_id = group_memberships.user_id AND scim_user_bindings.connection_id = ? AND scim_user_bindings.retired_at IS NULL", connectionID).
			Where("group_memberships.group_id IN ?", batch).
			Order("scim_user_bindings.created_at, scim_user_bindings.id").
			Scan(&batchRows).Error; err != nil {
			return nil, fmt.Errorf("failed to list SCIM group members: %w", err)
		}
		rows = append(rows, batchRows...)
	}

	for i := range rows {
		if err := c.decryptSCIMUserBinding(ctx, &rows[i].SCIMUserBinding); err != nil {
			return nil, err
		}
		members[rows[i].GroupID] = append(members[rows[i].GroupID], SCIMGroupMember{
			ID:       rows[i].ID,
			UserName: rows[i].UserName,
		})
	}
	return members, nil
}

// resolveSCIMMembersTx returns the Obot user IDs of the SCIM users in memberIDs. A value that refers to a user whose
// Obot account was deleted is ignored, because the identity provider may still list them. Any other value that is not
// a user of this connection fails the whole request.
func resolveSCIMMembersTx(tx *gorm.DB, connectionID string, memberIDs []string) (map[uint]struct{}, error) {
	ids := make([]string, 0, len(memberIDs))
	seen := make(map[string]struct{}, len(memberIDs))
	for _, id := range memberIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	users := make(map[uint]struct{}, len(ids))
	found := make(map[string]struct{}, len(ids))
	for batch := range slices.Chunk(ids, scimMemberBatchSize) {
		var bindings []types.SCIMUserBinding
		if err := tx.Select("id", "user_id", "retired_at").
			Where("connection_id = ? AND id IN ?", connectionID, batch).
			Find(&bindings).Error; err != nil {
			return nil, fmt.Errorf("failed to resolve SCIM group members: %w", err)
		}

		for _, binding := range bindings {
			found[binding.ID] = struct{}{}
			if !binding.Retired() {
				users[binding.UserID] = struct{}{}
			}
		}
	}

	for _, id := range ids {
		if _, ok := found[id]; ok {
			continue
		}

		// Retired group bindings are kept, so a group is recognized even after it was deleted in the target.
		var groups int64
		if err := tx.Model(new(types.SCIMGroupBinding)).
			Where("id = ? AND connection_id = ?", id, connectionID).
			Count(&groups).Error; err != nil {
			return nil, fmt.Errorf("failed to resolve SCIM group member %s: %w", id, err)
		} else if groups > 0 {
			return nil, &SCIMInvalidValueError{
				Message: fmt.Sprintf("member %q is a group; nested groups are not supported", id),
			}
		}
		return nil, &SCIMInvalidValueError{
			Message: fmt.Sprintf("member %q is not a user of this SCIM connection", id),
		}
	}

	return users, nil
}

// replaceGroupMembershipsTx makes users the complete member set of the group. It returns the users whose membership
// changed, each mapped to whether they left the group. Unchanged members are not touched.
func replaceGroupMembershipsTx(tx *gorm.DB, groupID string, users map[uint]struct{}) (map[uint]bool, error) {
	var current []uint
	if err := tx.Model(new(types.GroupMemberships)).Where("group_id = ?", groupID).Pluck("user_id", &current).Error; err != nil {
		return nil, fmt.Errorf("failed to list the memberships of group %s: %w", groupID, err)
	}

	added := maps.Clone(users)
	changes := make(map[uint]bool)
	var removed []uint
	for _, userID := range current {
		if _, ok := added[userID]; ok {
			delete(added, userID)
			continue
		}
		removed = append(removed, userID)
		changes[userID] = true
	}

	memberships := make([]types.GroupMemberships, 0, len(added))
	for userID := range added {
		memberships = append(memberships, types.GroupMemberships{
			UserID:  userID,
			GroupID: groupID,
		})
		changes[userID] = false
	}
	slices.SortFunc(memberships, func(a, b types.GroupMemberships) int {
		return cmp.Compare(a.UserID, b.UserID)
	})

	for batch := range slices.Chunk(removed, scimMemberBatchSize) {
		if err := tx.Where("group_id = ? AND user_id IN ?", groupID, batch).Delete(new(types.GroupMemberships)).Error; err != nil {
			return nil, fmt.Errorf("failed to remove members of group %s: %w", groupID, err)
		}
	}
	for batch := range slices.Chunk(memberships, scimMemberBatchSize) {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&batch).Error; err != nil {
			return nil, fmt.Errorf("failed to add members to group %s: %w", groupID, err)
		}
	}

	return changes, nil
}

// recordMembershipReconcileEventsTx records a reconcile event for each user whose memberships changed, noting whether
// they left a group. The caller must kick lifecycle delivery after committing when there were changes.
func recordMembershipReconcileEventsTx(tx *gorm.DB, changes map[uint]bool) error {
	for _, userID := range slices.Sorted(maps.Keys(changes)) {
		if err := recordUserReconcileEvent(tx, userID, changes[userID]); err != nil {
			return err
		}
	}
	return nil
}

// checkSCIMGroupNameTx fails if another bound group of the connection has the normalized name. Unbound groups never
// conflict: they are only candidates for binding.
func checkSCIMGroupNameTx(tx *gorm.DB, connectionID, normalized, displayName, exceptID string) error {
	query := tx.Model(new(types.SCIMGroupBinding)).
		Where("connection_id = ? AND retired_at IS NULL AND normalized_display_name = ?", connectionID, normalized)
	if exceptID != "" {
		query = query.Where("id != ?", exceptID)
	}

	var conflicts int64
	if err := query.Count(&conflicts).Error; err != nil {
		return fmt.Errorf("failed to check SCIM group name: %w", err)
	} else if conflicts > 0 {
		return &SCIMConflictError{
			Message: fmt.Sprintf("a group named %q already exists", displayName),
		}
	}
	return nil
}

// unboundGroupsNamedTx returns the groups of the connection's auth provider that have no unretired binding and whose
// normalized name is normalized. Groups pending deletion for being unreferenced are not candidates: they grant nothing,
// and are about to be deleted. Names are compared in Go, because SQLite does not case-fold non-ASCII characters.
func unboundGroupsNamedTx(tx *gorm.DB, conn *types.SCIMConnection, normalized string) ([]types.Group, error) {
	var groups []types.Group
	if err := tx.Select("id", "name").
		Where("auth_provider_namespace = ? AND auth_provider_name = ?", conn.AuthProviderNamespace, conn.AuthProviderName).
		Where("id NOT IN (?)", tx.Model(new(types.SCIMGroupBinding)).Select("group_id").Where("retired_at IS NULL")).
		Where("id NOT IN (?)", tx.Model(new(types.SCIMPendingGroupDeletion)).Select("group_id")).
		Order("id").
		Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("failed to list unbound groups: %w", err)
	}

	matches := make([]types.Group, 0, 1)
	for _, group := range groups {
		if types.NormalizeSCIMName(group.Name) == normalized {
			matches = append(matches, group)
		}
	}
	return matches, nil
}

func renameGroupTx(tx *gorm.DB, groupID, name string) error {
	if err := tx.Model(new(types.Group)).Where("id = ?", groupID).Update("name", name).Error; err != nil {
		return fmt.Errorf("failed to rename group %s: %w", groupID, err)
	}
	return nil
}

func scimGroupQuery(tx *gorm.DB, connectionID string) *gorm.DB {
	return tx.Table("scim_group_bindings").
		Joins("JOIN groups ON groups.id = scim_group_bindings.group_id").
		Where("scim_group_bindings.connection_id = ? AND scim_group_bindings.retired_at IS NULL", connectionID)
}

func scimGroupFromRow(row scimGroupRow, members map[string][]SCIMGroupMember, includeMembers bool) SCIMGroup {
	group := SCIMGroup{
		ID:          row.ID,
		GroupID:     row.GroupID,
		DisplayName: row.Name,
		Revision:    row.Revision,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if includeMembers {
		group.Members = members[row.GroupID]
		if group.Members == nil {
			group.Members = []SCIMGroupMember{}
		}
	}
	return group
}
