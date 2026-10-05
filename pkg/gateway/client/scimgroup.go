package client

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// scimMemberBatchSize bounds the number of parameters in a single membership query.
	scimMemberBatchSize = 500

	// scimGroupSubjectCleanupClaimDuration is how long a replica holds a cleanup of group subjects it is running. It
	// covers a run, which waits up to scimReferenceWriteLifetime for writes of references, and then reads and updates
	// the policies of each namespace once. A failed cleanup keeps its claim, so this is also the delay before it is
	// retried.
	scimGroupSubjectCleanupClaimDuration = 5 * time.Minute
)

// SCIMGroupInput holds the writable attributes of a SCIM group, as a create or a full replacement sends them.
type SCIMGroupInput struct {
	DisplayName string
	// MemberIDs are the SCIM user IDs of the complete member set.
	MemberIDs []string
}

// SCIMGroupPatch changes a bound group by naming the members it adds and removes, rather than its complete member
// set, so that applying it never reads the group's other members.
type SCIMGroupPatch struct {
	// DisplayName renames the group unless it is empty.
	DisplayName string
	// ReplaceMembers makes MemberIDs the complete member set. Otherwise, the users in MemberIDs are added, those in
	// RemovedMemberIDs are removed, and other members are not touched. All are SCIM user IDs.
	ReplaceMembers   bool
	MemberIDs        []string
	RemovedMemberIDs []string
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
			query = query.Where("scim_group_bindings.normalized_display_name = ?", types.NormalizeSCIMGroupName(filter.DisplayName))
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

	normalized := types.NormalizeSCIMGroupName(input.DisplayName)
	if normalized == "" {
		return nil, &SCIMInvalidValueError{
			Message: "displayName is required",
		}
	}

	var (
		bindingID string
		changed   bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if conn, err = lockSCIMConnectionWrites(tx, conn.ID); err != nil {
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
				Message: fmt.Sprintf("%d existing groups are named %q; in Obot, remove the references to all but one of them, and delete the groups that nothing references", len(candidates), input.DisplayName),
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
		bindingID = binding.ID
		return nil
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	// The group is read once the write lock is released, so that reading its members holds up no other SCIM write.
	return c.GetSCIMGroup(ctx, conn.ID, bindingID, true)
}

// UpdateSCIMGroup changes a bound group's display name and members. The replacement is computed by mutate from the
// group's current state, with its members, inside the transaction that writes it. A replacement identical to the
// current state changes nothing and emits nothing. A replacement that does not depend on the current state is a
// PatchSCIMGroup that replaces the members, which reads none of them.
func (c *Client) UpdateSCIMGroup(ctx context.Context, conn *types.SCIMConnection, id string, mutate func(current SCIMGroup) (SCIMGroupInput, error)) error {
	var changed bool
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

		normalized := types.NormalizeSCIMGroupName(input.DisplayName)
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
		return nil
	}); err != nil {
		return err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}
	return nil
}

// PatchSCIMGroup applies a patch to a bound group. Unlike UpdateSCIMGroup, it never loads the group's members: adding
// and removing members reads and writes only the memberships of the users the patch names, and replacing them reads
// only the user IDs of the group's memberships. A patch that changes nothing emits nothing.
func (c *Client) PatchSCIMGroup(ctx context.Context, conn *types.SCIMConnection, id string, patch SCIMGroupPatch) error {
	var changed bool
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		current, err := c.scimGroupTx(ctx, tx, conn.ID, id, false, true)
		if err != nil {
			return err
		}

		renamed := patch.DisplayName != "" && patch.DisplayName != current.DisplayName
		if renamed {
			normalized := types.NormalizeSCIMGroupName(patch.DisplayName)
			if normalized == "" {
				return &SCIMInvalidValueError{
					Message: "displayName is required",
				}
			}
			if err := checkSCIMGroupNameTx(tx, conn.ID, normalized, patch.DisplayName, id); err != nil {
				return err
			}
			if err := renameGroupTx(tx, current.GroupID, patch.DisplayName); err != nil {
				return err
			}
		}

		var changes map[uint]bool
		if patch.ReplaceMembers {
			members, err := resolveSCIMMembersTx(tx, conn.ID, patch.MemberIDs)
			if err != nil {
				return err
			}
			if changes, err = replaceGroupMembershipsTx(tx, current.GroupID, members); err != nil {
				return err
			}
		} else {
			added, err := resolveSCIMMembersTx(tx, conn.ID, patch.MemberIDs)
			if err != nil {
				return err
			}
			removed, err := scimMemberUserIDsTx(tx, conn.ID, patch.RemovedMemberIDs)
			if err != nil {
				return err
			}
			if changes, err = changeGroupMembershipsTx(tx, current.GroupID, added, removed); err != nil {
				return err
			}
		}
		if err := recordMembershipReconcileEventsTx(tx, changes); err != nil {
			return err
		}
		changed = len(changes) > 0

		if !renamed && !changed {
			return nil
		}
		columns := map[string]any{
			"revision":   gorm.Expr("revision + 1"),
			"updated_at": time.Now(),
		}
		if renamed {
			columns["normalized_display_name"] = types.NormalizeSCIMGroupName(patch.DisplayName)
		}
		if err := tx.Model(new(types.SCIMGroupBinding)).Where("id = ?", id).UpdateColumns(columns).Error; err != nil {
			return fmt.Errorf("failed to update SCIM group binding: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}
	return nil
}

// DeleteSCIMGroup retires a group's binding and removes its memberships. The retired binding stays, so that its SCIM
// ID is never reused.
//
// While SCIM is not enforced, the group and every reference to it remain, and the group becomes unbound again:
// pushing it later under the same name binds it under a new SCIM ID and restores its members. Once SCIM is enforced,
// the group is deleted as well, with its group role assignments, so that a group pushed later under the same name
// gets none of what the deleted group was granted. Its subjects in access policies are removed by a cleanup that
// ClaimSCIMGroupSubjectCleanups hands out.
func (c *Client) DeleteSCIMGroup(ctx context.Context, conn *types.SCIMConnection, id string) error {
	var changed bool
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Enforcing takes the same lock, so the state read here holds until the transaction commits.
		var err error
		if conn, err = lockSCIMConnectionWrites(tx, conn.ID); err != nil {
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

		if conn.State == types.SCIMConnectionStateEnforced {
			return deleteSCIMGroupTx(tx, conn, binding.GroupID)
		}
		return nil
	}); err != nil {
		return err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}
	return nil
}

// deleteSCIMGroupTx deletes a group that has no members left, with its group role assignments and any mark for its
// deletion, and records the cleanup of its subjects in access policies. A reference write that checked the group
// before this commits may still save a subject, which the cleanup waits for. Any later one is refused, because the
// group no longer exists.
func deleteSCIMGroupTx(tx *gorm.DB, conn *types.SCIMConnection, groupID string) error {
	if err := tx.Where("group_name = ?", groupID).Delete(new(types.GroupRoleAssignment)).Error; err != nil {
		return fmt.Errorf("failed to delete the group role assignments of group %s: %w", groupID, err)
	}
	if err := tx.Where("group_id = ?", groupID).Delete(new(types.SCIMPendingGroupDeletion)).Error; err != nil {
		return fmt.Errorf("failed to delete the mark for deletion of group %s: %w", groupID, err)
	}
	if err := deleteGroupsTx(tx, conn, []string{groupID}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&types.SCIMGroupSubjectCleanup{
		GroupID:   groupID,
		Namespace: conn.AuthProviderNamespace,
	}).Error; err != nil {
		return fmt.Errorf("failed to record the cleanup of the subjects of group %s: %w", groupID, err)
	}
	return nil
}

// ClaimSCIMGroupSubjectCleanups claims, for this replica, up to limit cleanups of the subjects of deleted groups that
// no replica is running, and returns them. The caller completes or fails each one.
func (c *Client) ClaimSCIMGroupSubjectCleanups(ctx context.Context, limit int) ([]types.SCIMGroupSubjectCleanup, error) {
	now := time.Now()
	var cleanups []types.SCIMGroupSubjectCleanup
	if err := c.db.WithContext(ctx).
		Where("claimed_until IS NULL OR claimed_until < ?", now).
		Order("created_at, group_id").
		Limit(limit).
		Find(&cleanups).Error; err != nil {
		return nil, fmt.Errorf("failed to list cleanups of group subjects: %w", err)
	}

	claimed := make([]types.SCIMGroupSubjectCleanup, 0, len(cleanups))
	for _, cleanup := range cleanups {
		result := c.db.WithContext(ctx).Model(new(types.SCIMGroupSubjectCleanup)).
			Where("group_id = ? AND (claimed_until IS NULL OR claimed_until < ?)", cleanup.GroupID, now).
			UpdateColumn("claimed_until", now.Add(scimGroupSubjectCleanupClaimDuration))
		if result.Error != nil {
			return nil, fmt.Errorf("failed to claim the cleanup of the subjects of group %s: %w", cleanup.GroupID, result.Error)
		}
		// Otherwise, another replica claimed it first.
		if result.RowsAffected == 1 {
			claimed = append(claimed, cleanup)
		}
	}
	return claimed, nil
}

// CompleteSCIMGroupSubjectCleanup records that the subjects of a deleted group were removed.
func (c *Client) CompleteSCIMGroupSubjectCleanup(ctx context.Context, groupID string) error {
	if err := c.db.WithContext(ctx).Where("group_id = ?", groupID).Delete(new(types.SCIMGroupSubjectCleanup)).Error; err != nil {
		return fmt.Errorf("failed to complete the cleanup of the subjects of group %s: %w", groupID, err)
	}
	return nil
}

// FailSCIMGroupSubjectCleanup records why removing the subjects of a deleted group failed. The cleanup keeps its
// claim, so it is retried once the claim expires.
func (c *Client) FailSCIMGroupSubjectCleanup(ctx context.Context, groupID string, cause error) error {
	if err := c.db.WithContext(ctx).Model(new(types.SCIMGroupSubjectCleanup)).
		Where("group_id = ?", groupID).
		UpdateColumns(map[string]any{
			"attempts":   gorm.Expr("attempts + 1"),
			"last_error": cause.Error(),
		}).Error; err != nil {
		return fmt.Errorf("failed to record the failed cleanup of the subjects of group %s: %w", groupID, err)
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

	// A member needs only its SCIM ID and userName, so only those are read, and only the userName is decrypted.
	type memberRow struct {
		ID        string
		UserName  string
		Encrypted bool
		GroupID   string
	}
	var rows []memberRow
	for batch := range slices.Chunk(groupIDs, scimMemberBatchSize) {
		var batchRows []memberRow
		if err := tx.Table("group_memberships").
			Select("scim_user_bindings.id AS id, scim_user_bindings.user_name AS user_name, scim_user_bindings.encrypted AS encrypted, group_memberships.group_id AS group_id").
			Joins("JOIN scim_user_bindings ON scim_user_bindings.user_id = group_memberships.user_id AND scim_user_bindings.connection_id = ? AND scim_user_bindings.retired_at IS NULL", connectionID).
			Where("group_memberships.group_id IN ?", batch).
			Order("scim_user_bindings.created_at, scim_user_bindings.id").
			Scan(&batchRows).Error; err != nil {
			return nil, fmt.Errorf("failed to list SCIM group members: %w", err)
		}
		rows = append(rows, batchRows...)
	}

	for _, row := range rows {
		userName := row.UserName
		if row.Encrypted {
			var err error
			if userName, err = c.decryptSCIMUserBindingField(ctx, row.ID, userName); err != nil {
				return nil, err
			}
		}
		members[row.GroupID] = append(members[row.GroupID], SCIMGroupMember{
			ID:       row.ID,
			UserName: userName,
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
	for _, id := range scimIDs(memberIDs) {
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

// scimMemberUserIDsTx returns the Obot user IDs of the connection's live SCIM users in memberIDs. Unlike
// resolveSCIMMembersTx, it ignores values that are not users of the connection, which removing changes nothing for.
func scimMemberUserIDsTx(tx *gorm.DB, connectionID string, memberIDs []string) (map[uint]struct{}, error) {
	users := make(map[uint]struct{}, len(memberIDs))
	for batch := range slices.Chunk(scimIDs(memberIDs), scimMemberBatchSize) {
		var userIDs []uint
		if err := tx.Model(new(types.SCIMUserBinding)).
			Where("connection_id = ? AND retired_at IS NULL AND id IN ?", connectionID, batch).
			Pluck("user_id", &userIDs).Error; err != nil {
			return nil, fmt.Errorf("failed to resolve SCIM group members: %w", err)
		}
		for _, userID := range userIDs {
			users[userID] = struct{}{}
		}
	}
	return users, nil
}

// scimIDs returns member values as the SCIM IDs they refer to. SCIM IDs are lowercase UUIDs, and member values match
// them case-insensitively, as the filters that select members do.
func scimIDs(memberIDs []string) []string {
	ids := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		ids = append(ids, strings.ToLower(id))
	}
	return ids
}

// replaceGroupMembershipsTx makes users the complete member set of the group. It returns the users whose membership
// changed, each mapped to whether they left the group. Unchanged members are not touched.
func replaceGroupMembershipsTx(tx *gorm.DB, groupID string, users map[uint]struct{}) (map[uint]bool, error) {
	var current []uint
	if err := tx.Model(new(types.GroupMemberships)).Where("group_id = ?", groupID).Pluck("user_id", &current).Error; err != nil {
		return nil, fmt.Errorf("failed to list the memberships of group %s: %w", groupID, err)
	}

	changes := make(map[uint]bool)
	members := make(map[uint]struct{}, len(current))
	for _, userID := range current {
		members[userID] = struct{}{}
		if _, ok := users[userID]; !ok {
			changes[userID] = true
		}
	}
	for userID := range users {
		if _, ok := members[userID]; !ok {
			changes[userID] = false
		}
	}

	return changes, writeGroupMembershipChangesTx(tx, groupID, changes)
}

// changeGroupMembershipsTx adds the added users to the group and removes the removed ones, reading only their
// memberships. A user in both is added. It returns the users whose membership changed, each mapped to whether they
// left the group.
func changeGroupMembershipsTx(tx *gorm.DB, groupID string, added, removed map[uint]struct{}) (map[uint]bool, error) {
	named := slices.Collect(maps.Keys(added))
	for userID := range removed {
		if _, ok := added[userID]; !ok {
			named = append(named, userID)
		}
	}

	members := make(map[uint]struct{}, len(named))
	for batch := range slices.Chunk(named, scimMemberBatchSize) {
		var userIDs []uint
		if err := tx.Model(new(types.GroupMemberships)).
			Where("group_id = ? AND user_id IN ?", groupID, batch).
			Pluck("user_id", &userIDs).Error; err != nil {
			return nil, fmt.Errorf("failed to list the memberships of group %s: %w", groupID, err)
		}
		for _, userID := range userIDs {
			members[userID] = struct{}{}
		}
	}

	changes := make(map[uint]bool, len(named))
	for _, userID := range named {
		_, member := members[userID]
		_, adding := added[userID]
		if adding != member {
			changes[userID] = member
		}
	}

	return changes, writeGroupMembershipChangesTx(tx, groupID, changes)
}

// writeGroupMembershipChangesTx removes the users that changes maps to true from the group, and adds those it maps to
// false.
func writeGroupMembershipChangesTx(tx *gorm.DB, groupID string, changes map[uint]bool) error {
	var (
		removed     []uint
		memberships []types.GroupMemberships
	)
	for _, userID := range slices.Sorted(maps.Keys(changes)) {
		if changes[userID] {
			removed = append(removed, userID)
			continue
		}
		memberships = append(memberships, types.GroupMemberships{
			UserID:  userID,
			GroupID: groupID,
		})
	}

	for batch := range slices.Chunk(removed, scimMemberBatchSize) {
		if err := tx.Where("group_id = ? AND user_id IN ?", groupID, batch).Delete(new(types.GroupMemberships)).Error; err != nil {
			return fmt.Errorf("failed to remove members of group %s: %w", groupID, err)
		}
	}
	for batch := range slices.Chunk(memberships, scimMemberBatchSize) {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&batch).Error; err != nil {
			return fmt.Errorf("failed to add members to group %s: %w", groupID, err)
		}
	}
	return nil
}

// recordMembershipReconcileEventsTx records a reconcile event for each user whose memberships changed, noting whether
// they left a group, in batches. The caller must kick lifecycle delivery after committing when there were changes.
func recordMembershipReconcileEventsTx(tx *gorm.DB, changes map[uint]bool) error {
	if len(changes) == 0 {
		return nil
	}

	events := make([]types.UserLifecycleEvent, 0, len(changes))
	for _, userID := range slices.Sorted(maps.Keys(changes)) {
		events = append(events, types.UserLifecycleEvent{
			UserID:        userID,
			Type:          types.UserLifecycleEventReconcile,
			GroupsRemoved: changes[userID],
		})
	}
	if err := tx.CreateInBatches(&events, scimMemberBatchSize).Error; err != nil {
		return fmt.Errorf("failed to record the reconcile events of %d users: %w", len(events), err)
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
		if types.NormalizeSCIMGroupName(group.Name) == normalized {
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
