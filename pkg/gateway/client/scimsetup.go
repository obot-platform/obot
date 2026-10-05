package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// SCIMEnforceActorOtherAuthProvider means the actor did not sign in through the connection's auth provider.
	SCIMEnforceActorOtherAuthProvider SCIMEnforceActorProblem = "otherAuthProvider"
	// SCIMEnforceActorNotSignedIn means the actor has no identity of the connection's auth provider that has signed
	// in.
	SCIMEnforceActorNotSignedIn SCIMEnforceActorProblem = "notSignedIn"
	// SCIMEnforceActorUnprovisioned means the connection has not provisioned the actor.
	SCIMEnforceActorUnprovisioned SCIMEnforceActorProblem = "unprovisioned"
	// SCIMEnforceActorDeactivated means the identity provider deactivated the actor.
	SCIMEnforceActorDeactivated SCIMEnforceActorProblem = "deactivated"
	// SCIMEnforceActorDisabled means the actor is disabled or deleted in Obot.
	SCIMEnforceActorDisabled SCIMEnforceActorProblem = "disabled"

	// scimReferenceWriteLifetime is how long a write of group references counts as in progress. A write must finish
	// within it; one that never finishes stops holding up deletions of unreferenced groups once it expires.
	scimReferenceWriteLifetime = time.Minute
	// scimReferenceWritePollInterval is how often a deletion checks whether the writes it waits for have finished.
	scimReferenceWritePollInterval = 50 * time.Millisecond
	// scimDeletionMarkLifetime is how long a mark for the deletion of an unreferenced group lasts. A deletion
	// finishes well within it: it waits at most scimReferenceWriteLifetime for writes of references, then reads the
	// references and deletes.
	scimDeletionMarkLifetime = 10 * time.Minute
	// scimDeletionMarkExpiryInterval is how often expired marks for deletion are removed.
	scimDeletionMarkExpiryInterval = time.Minute

	// signedInSQL selects whether the user has an identity of the auth provider, the query's two parameters, that
	// has signed in.
	signedInSQL = "EXISTS (SELECT 1 FROM identities WHERE identities.user_id = users.id AND identities.auth_provider_namespace = ? AND identities.auth_provider_name = ? AND identities.first_sign_in_at IS NOT NULL)"
)

// SCIMProviderGroup is a group of an auth provider, with its SCIM binding if it has one.
type SCIMProviderGroup struct {
	ID   string
	Name string
	// SCIMID is the ID of the group's unretired SCIM binding. It is empty while the group is unbound.
	SCIMID string `gorm:"column:scim_id"`
	// PendingDeletion is set while the group is marked for deletion because nothing references it.
	PendingDeletion bool `gorm:"column:pending_deletion"`
	// PendingRunID identifies the deletion that marked the group.
	PendingRunID string `gorm:"column:pending_run_id"`
}

// SCIMDeletionRun is one deletion of the unreferenced groups of a SCIM connection's auth provider. Only the deletion
// that marked a group deletes it.
type SCIMDeletionRun struct {
	ID string
	// GroupIDs are the groups the deletion marked.
	GroupIDs []string
}

// AuthProviderGroupData is the group data of an auth provider in the gateway database.
type AuthProviderGroupData struct {
	// Groups are the provider's groups.
	Groups []SCIMProviderGroup
	// MembershipCount is the number of memberships in groups with the provider's group ID prefix.
	MembershipCount int64
	// RoleAssignmentGroupIDs are the group IDs with the provider's group ID prefix that a group role assignment
	// names.
	RoleAssignmentGroupIDs []string
}

// SCIMResidualGroupDataError reports that a SCIM connection was not created, because its auth provider still has
// group data from an earlier configuration. The provider's auth provider cleanup removes it.
type SCIMResidualGroupDataError struct {
	AuthProviderName string
	Data             AuthProviderGroupData
}

// SCIMSetupUser is a user of a SCIM connection's auth provider.
type SCIMSetupUser struct {
	UserID         uint
	Username       string
	Email          string
	DisplayName    string
	DisabledAt     *time.Time
	DisabledReason types.UserDisabledReason
	// SCIMID is the ID of the user's unretired SCIM binding. It is empty while the user is unprovisioned.
	SCIMID string
	// Active is the provisioned state the identity provider last sent.
	Active bool
	// SignedIn is set once the user has signed in through the connection's auth provider.
	SignedIn bool
}

// SCIMEnforceActor is the user asking to enforce SCIM, and the auth provider they signed in with.
type SCIMEnforceActor struct {
	UserID                uint
	AuthProviderNamespace string
	AuthProviderName      string
}

// SCIMEnforceActorProblem says why a user may not enforce SCIM: they could not be known to sign in once it is
// enforced.
type SCIMEnforceActorProblem string

// EnforceSCIMOptions holds what enforcing a SCIM connection needs from outside the gateway database.
type EnforceSCIMOptions struct {
	// RunID identifies the deletion that marked the unreferenced groups. Enforcing deletes only the groups it marked.
	RunID string
	// ReferencedGroupIDs are the IDs of the groups that anything references, read after the deletion's marks
	// committed and the reference writes in progress then finished.
	ReferencedGroupIDs map[string]struct{}
	Actor              SCIMEnforceActor
}

// EnforceSCIMResult is what enforcing a SCIM connection did.
type EnforceSCIMResult struct {
	Connection *types.SCIMConnection
	// DisabledUserIDs are the users disabled because SCIM never provisioned them.
	DisabledUserIDs []uint
	// DeletedGroupIDs are the unbound groups deleted because nothing referenced them.
	DeletedGroupIDs []string
}

// SCIMEnforceBlockedError reports why a SCIM connection cannot be enforced yet. Enforcing clears its marks for
// deletion when it is blocked.
type SCIMEnforceBlockedError struct {
	// UnboundGroups are the referenced groups that the identity provider has not pushed.
	UnboundGroups []SCIMProviderGroup
	// ActorProblem says why the acting user may not enforce SCIM, and is empty when they may.
	ActorProblem SCIMEnforceActorProblem
}

// SCIMConnectionStateError reports a transition that the connection's state does not allow. Both transitions are
// permanent, so neither can be repeated or reversed.
type SCIMConnectionStateError struct {
	State types.SCIMConnectionState
}

// SCIMGroupReferenceError reports new references to groups that a SCIM connection's auth provider cannot use: group
// IDs with the connection's group ID prefix that no group of the provider has, and groups that are being deleted
// because nothing referenced them.
type SCIMGroupReferenceError struct {
	AuthProviderNamespace string
	AuthProviderName      string
	// Missing are group IDs that no group of the provider has.
	Missing []string
	// PendingDeletion are groups that are being deleted.
	PendingDeletion []string
}

// DeletedSCIMConnection is what deleting the SCIM connection of an auth provider did.
type DeletedSCIMConnection struct {
	ConnectionID string
}

type scimSetupUserRow struct {
	types.User
	SCIMID   string `gorm:"column:scim_id"`
	Active   bool   `gorm:"column:scim_active"`
	SignedIn bool   `gorm:"column:signed_in"`
}

func (e *SCIMResidualGroupDataError) Error() string {
	return fmt.Sprintf("auth provider %q still has %d groups, %d memberships, and %d group role assignments from an earlier configuration",
		e.AuthProviderName, len(e.Data.Groups), e.Data.MembershipCount, len(e.Data.RoleAssignmentGroupIDs))
}

func (e *SCIMEnforceBlockedError) Error() string {
	var problems []string
	if len(e.UnboundGroups) > 0 {
		problems = append(problems, fmt.Sprintf("%d referenced groups are not bound", len(e.UnboundGroups)))
	}
	if e.ActorProblem != "" {
		problems = append(problems, string(e.ActorProblem))
	}
	return "SCIM cannot be enforced: " + strings.Join(problems, "; ")
}

func (e *SCIMConnectionStateError) Error() string {
	return fmt.Sprintf("the SCIM connection is already %s", e.State)
}

func (e *SCIMGroupReferenceError) Error() string {
	var problems []string
	if len(e.Missing) > 0 {
		problems = append(problems, fmt.Sprintf("no group of auth provider %s has the ID %s", e.AuthProviderName, strings.Join(e.Missing, ", ")))
	}
	if len(e.PendingDeletion) > 0 {
		problems = append(problems, fmt.Sprintf("groups %s of auth provider %s are being deleted", strings.Join(e.PendingDeletion, ", "), e.AuthProviderName))
	}
	return strings.Join(problems, "; ")
}

// Empty reports whether the auth provider has no group data at all.
func (d AuthProviderGroupData) Empty() bool {
	return len(d.Groups) == 0 && d.MembershipCount == 0 && len(d.RoleAssignmentGroupIDs) == 0
}

// AuthProviderGroupData returns the group data of an auth provider: its groups, and the memberships and group role
// assignments of group IDs with its group ID prefix. Auth provider cleanup deletes exactly this data.
func (c *Client) AuthProviderGroupData(ctx context.Context, namespace, name, groupIDPrefix string) (*AuthProviderGroupData, error) {
	return authProviderGroupDataTx(c.db.WithContext(ctx), namespace, name, groupIDPrefix)
}

// SCIMProviderGroups returns the groups of the auth provider, bound and unbound, ordered by name.
func (c *Client) SCIMProviderGroups(ctx context.Context, namespace, name string) ([]SCIMProviderGroup, error) {
	return scimProviderGroupsTx(c.db.WithContext(ctx), namespace, name)
}

// DeleteAuthProviderSCIMConnection deletes the SCIM connection of an auth provider that is being deconfigured, with
// all of its SCIM data and the provider's group data: its groups, and the memberships and group role assignments of
// group IDs with its group ID prefix. The provider's users and identities are kept. Users that SCIM disabled stay
// disabled, with their API keys and agents, until an administrator enables them: enabling them here would give their
// API keys, and the agents that mint keys for them, access again without any sign-in. Configuring the provider again
// starts SCIM over, with a new connection that the identity provider pushes its users and groups to again.
//
// It records no reconcile events: the auth provider cleanup removes the provider's groups from access policies and
// reconciles every user with an identity of the provider. It returns nil, and changes nothing, when the provider has
// no connection, so that a retried deconfiguration is safe.
func (c *Client) DeleteAuthProviderSCIMConnection(ctx context.Context, provider AuthProviderRef) (*DeletedSCIMConnection, error) {
	return c.deleteSCIMConnection(ctx, provider, nil)
}

// DeleteStagedSCIMConnection deletes the SCIM connection that staging an auth provider without directory credentials
// created, as DeleteAuthProviderSCIMConnection does, when the staging is discarded. It deletes nothing, and returns
// nil, unless the provider holds a staged credential and no credential in its own or the generic auth provider
// context: deleting the connection of the configured provider would stop its sign-ins.
func (c *Client) DeleteStagedSCIMConnection(ctx context.Context, provider AuthProviderRef) (*DeletedSCIMConnection, error) {
	return c.deleteSCIMConnection(ctx, provider, func(tx *gorm.DB) (bool, error) {
		staged, err := hasCredentialTx(tx, []string{system.ReplacementAuthProviderCredentialContext}, provider.Name)
		if err != nil || !staged {
			return false, err
		}
		active, err := hasCredentialTx(tx, []string{provider.Name, system.GenericAuthProviderCredentialContext}, provider.Name)
		return !active, err
	})
}

// deleteSCIMConnection deletes the SCIM connection of the auth provider as DeleteAuthProviderSCIMConnection says,
// if it has one and allowed, when given, reports that it may be deleted.
func (c *Client) deleteSCIMConnection(ctx context.Context, provider AuthProviderRef, allowed func(*gorm.DB) (bool, error)) (*DeletedSCIMConnection, error) {
	var deleted *DeletedSCIMConnection
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Sign-ins and SCIM writes either finish before the connection is deleted, or find it gone.
		if err := lockSCIMMode(tx, true); err != nil {
			return err
		}
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		conn, err := scimConnectionForAuthProviderTx(tx, provider.Namespace, provider.Name)
		if err != nil || conn == nil {
			return err
		}
		// Token changes and recorded request failures lock the connection row, and take neither advisory lock.
		if conn, err = scimConnectionTx(tx, conn.ID, true); err != nil {
			return err
		}
		if allowed != nil {
			if ok, err := allowed(tx); err != nil || !ok {
				return err
			}
		}

		if err := deleteSCIMConnectionTx(tx, conn); err != nil {
			return err
		}
		deleted = &DeletedSCIMConnection{
			ConnectionID: conn.ID,
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to delete the SCIM connection of auth provider %s/%s: %w", provider.Namespace, provider.Name, err)
	}
	return deleted, nil
}

// MarkUnreferencedSCIMGroups starts a deletion of unreferenced groups: it marks the unbound groups of the connection's
// auth provider whose IDs are not in referenced as pending deletion, and returns once every write of group references
// that could have missed the marks has finished.
//
// The references that decide which groups are deleted live in the controller store, which cannot share a
// transaction with the gateway database. Writers of references record their write before they check for marks. From
// the moment the marks commit, a write recorded afterwards sees them and is refused, and a pushed group no longer
// binds to a marked group. The writes recorded by then are waited for, so the references read after this returns
// include theirs, and are the last that can reach the marked groups.
//
// A group that another deletion marked is taken over. Only the deletion that marked a group deletes it, once it has
// read the references again.
func (c *Client) MarkUnreferencedSCIMGroups(ctx context.Context, conn *types.SCIMConnection, referenced map[string]struct{}) (*SCIMDeletionRun, error) {
	run := &SCIMDeletionRun{
		ID: uuid.NewV4().String(),
	}
	// Marks expire by the clock that the expiry reads.
	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A pushed group either binds before its group is marked, and is kept, or sees the mark.
		var err error
		if conn, err = lockSCIMConnectionWrites(tx, conn.ID); err != nil {
			return err
		}

		groups, err := scimProviderGroupsTx(tx, conn.AuthProviderNamespace, conn.AuthProviderName)
		if err != nil {
			return err
		}

		marks := make([]types.SCIMPendingGroupDeletion, 0, len(groups))
		for _, group := range groups {
			if _, ok := referenced[group.ID]; ok || group.SCIMID != "" {
				continue
			}
			marks = append(marks, types.SCIMPendingGroupDeletion{
				GroupID:      group.ID,
				ConnectionID: conn.ID,
				RunID:        run.ID,
				CreatedAt:    now,
			})
			run.GroupIDs = append(run.GroupIDs, group.ID)
		}
		for batch := range slices.Chunk(marks, scimMemberBatchSize) {
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{
						Name: "group_id",
					},
				},
				DoUpdates: clause.AssignmentColumns([]string{"run_id", "created_at"}),
			}).Create(&batch).Error; err != nil {
				return fmt.Errorf("failed to mark unreferenced groups for deletion: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if len(run.GroupIDs) > 0 {
		if err := c.WaitForSCIMReferenceWrites(ctx); err != nil {
			return run, err
		}
	}
	return run, nil
}

// runSCIMGroupDeletionMarkExpiry removes expired marks for deletion until ctx is done. It starts at once, so that the
// marks of a deletion that a restart interrupted expire without waiting for the first interval.
func (c *Client) runSCIMGroupDeletionMarkExpiry(ctx context.Context) {
	timer := time.NewTimer(scimDeletionMarkExpiryInterval)
	defer timer.Stop()

	for {
		if err := c.expireSCIMGroupDeletionMarks(ctx); err != nil && ctx.Err() == nil {
			slog.Error("Failed to remove expired marks for the deletion of unreferenced SCIM groups", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		timer.Reset(scimDeletionMarkExpiryInterval)
	}
}

// expireSCIMGroupDeletionMarks removes the marks for deletion that are older than scimDeletionMarkLifetime, so that
// references to their groups are accepted again. It holds the SCIM write lock, as a deletion does while it reads its
// marks and deletes their groups, so that a mark never expires in between: a reference accepted once it expired
// could then be to a deleted group.
func (c *Client) expireSCIMGroupDeletionMarks(ctx context.Context) error {
	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return err
	}
	// Most runs find nothing to expire, and take no lock.
	var marks int64
	if err := c.db.WithContext(ctx).Model(new(types.SCIMPendingGroupDeletion)).
		Where("created_at < ?", now.Add(-scimDeletionMarkLifetime)).
		Count(&marks).Error; err != nil {
		return fmt.Errorf("failed to count expired marks for deletion: %w", err)
	}
	if marks == 0 {
		return nil
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}
		if err := tx.Where("created_at < ?", now.Add(-scimDeletionMarkLifetime)).Delete(new(types.SCIMPendingGroupDeletion)).Error; err != nil {
			return fmt.Errorf("failed to remove expired marks for deletion: %w", err)
		}
		return nil
	})
}

// SCIMGroupDeletionInProgress reports whether a deletion of the connection's unreferenced groups holds marks for
// deletion that have not expired.
func (c *Client) SCIMGroupDeletionInProgress(ctx context.Context, connectionID string) (bool, error) {
	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return false, err
	}
	return otherSCIMGroupDeletionMarksTx(c.db.WithContext(ctx), connectionID, "", now)
}

// otherSCIMGroupDeletionMarksTx reports whether a deletion of the connection's unreferenced groups other than the run
// runID holds marks for deletion that have not expired by now.
func otherSCIMGroupDeletionMarksTx(tx *gorm.DB, connectionID, runID string, now time.Time) (bool, error) {
	var marks int64
	if err := tx.Model(new(types.SCIMPendingGroupDeletion)).
		Where("connection_id = ? AND run_id != ? AND created_at >= ?", connectionID, runID, now.Add(-scimDeletionMarkLifetime)).
		Count(&marks).Error; err != nil {
		return false, fmt.Errorf("failed to check for other deletions of unreferenced groups: %w", err)
	}
	return marks > 0, nil
}

// ClearSCIMGroupDeletionMarks unmarks the groups that a deletion marked and still holds, so that references to them
// are accepted again. It is used when the deletion does not go ahead.
func (c *Client) ClearSCIMGroupDeletionMarks(ctx context.Context, connectionID, runID string) error {
	return clearSCIMGroupDeletionMarksTx(c.db.WithContext(ctx), connectionID, runID)
}

// DeleteMarkedSCIMGroups finishes a deletion of unreferenced groups that MarkUnreferencedSCIMGroups started as the run
// runID, and returns the IDs of the groups it deleted. referenced holds the IDs of the groups that anything references,
// read after MarkUnreferencedSCIMGroups returned.
//
// A marked group that gained a reference in between is unmarked and kept, and so is one that SCIM bound or another
// deletion took over. The rest of the run's groups are deleted with their memberships. They grant nothing, so their
// deletion records no reconcile event, and triggers no cleanup of the resources of their former members.
func (c *Client) DeleteMarkedSCIMGroups(ctx context.Context, connectionID, runID string, referenced map[string]struct{}) ([]string, error) {
	var deleted []string
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A pushed group either binds before the groups are read here, and is kept, or waits for the deletion.
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		conn, err := scimConnectionTx(tx, connectionID, true)
		if err != nil {
			return err
		}
		deleted, err = finishSCIMGroupDeletionTx(tx, conn, runID, referenced)
		return err
	}); err != nil {
		return nil, err
	}
	return deleted, nil
}

// EnforceSCIMConnection requires a SCIM binding for sign-in with the connection's auth provider, permanently.
//
// The unreferenced groups must already be marked for deletion by the run opts.RunID, and opts.ReferencedGroupIDs read
// after MarkUnreferencedSCIMGroups returned. A marked group that gained a reference in between is unmarked and kept,
// and only the groups that run still holds marks on are deleted.
//
// Enforcing is refused with *SCIMEnforceBlockedError while a referenced group of the provider is unbound, because the
// memberships of a group the identity provider never pushes would be frozen forever, or while the actor could not
// be known to sign in afterwards. A refused Enforce clears the connection's marks for deletion. It is refused with
// ErrSCIMGroupDeletionInProgress, and changes nothing, while another deletion holds marks that have not expired.
//
// Otherwise, in one transaction, it disables every live user of the provider that the connection has not
// provisioned, deletes the groups still marked for deletion with their memberships, and records the enforcement. It
// changes no memberships of the groups it keeps, deletes no user, and records no reconcile event: the groups it
// deletes grant nothing.
func (c *Client) EnforceSCIMConnection(ctx context.Context, id string, opts EnforceSCIMOptions) (*EnforceSCIMResult, error) {
	var (
		result  = new(EnforceSCIMResult)
		blocked *SCIMEnforceBlockedError
	)
	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Sign-ins read the mode under the shared lock, so each one either completes before enforcement or sees it.
		// SCIM writes wait too, so a user is never provisioned between being found unprovisioned and being disabled.
		if err := lockSCIMMode(tx, true); err != nil {
			return err
		}
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		conn, err := scimConnectionTx(tx, id, true)
		if err != nil {
			return err
		}
		if conn.State != types.SCIMConnectionStateConnected {
			return &SCIMConnectionStateError{
				State: conn.State,
			}
		}
		// Enforcing clears every mark for deletion, so it would stop another deletion that is under way from deleting
		// what it marked, or that deletion took over marks of this one.
		if held, err := otherSCIMGroupDeletionMarksTx(tx, conn.ID, opts.RunID, now); err != nil {
			return err
		} else if held {
			return ErrSCIMGroupDeletionInProgress
		}

		groups, err := scimProviderGroupsTx(tx, conn.AuthProviderNamespace, conn.AuthProviderName)
		if err != nil {
			return err
		}
		var unbound []SCIMProviderGroup
		for _, group := range groups {
			if _, ok := opts.ReferencedGroupIDs[group.ID]; ok && group.SCIMID == "" {
				unbound = append(unbound, group)
			}
		}
		problem, err := checkSCIMEnforceActorTx(tx, conn, opts.Actor)
		if err != nil {
			return err
		}
		if len(unbound) > 0 || problem != "" {
			blocked = &SCIMEnforceBlockedError{
				UnboundGroups: unbound,
				ActorProblem:  problem,
			}
			return clearAllSCIMGroupDeletionMarksTx(tx, conn.ID)
		}

		var userIDs []uint
		if err := unprovisionedSCIMUsersQuery(tx, conn).Order("users.id").Pluck("users.id", &userIDs).Error; err != nil {
			return fmt.Errorf("failed to list unprovisioned users: %w", err)
		}
		provider := AuthProviderRef{
			Namespace: conn.AuthProviderNamespace,
			Name:      conn.AuthProviderName,
		}
		for _, userID := range userIDs {
			_, changed, err := disableUserTx(tx, provider, userID, types.UserDisabledReasonSCIMUnprovisioned)
			if err != nil {
				return err
			}
			if changed {
				result.DisabledUserIDs = append(result.DisabledUserIDs, userID)
			}
		}

		if result.DeletedGroupIDs, err = finishSCIMGroupDeletionTx(tx, conn, opts.RunID, opts.ReferencedGroupIDs); err != nil {
			return err
		}
		// Once SCIM is enforced, no other deletion of unreferenced groups is finished, so their marks go too.
		if err := clearAllSCIMGroupDeletionMarksTx(tx, conn.ID); err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(conn).UpdateColumns(map[string]any{
			"state":       types.SCIMConnectionStateEnforced,
			"enforced_at": now,
			"updated_at":  now,
		}).Error; err != nil {
			return fmt.Errorf("failed to enforce SCIM connection %s: %w", id, err)
		}
		conn.State = types.SCIMConnectionStateEnforced
		conn.EnforcedAt = &now
		conn.UpdatedAt = now
		result.Connection = conn
		return nil
	}); err != nil {
		return nil, err
	}
	if blocked != nil {
		return nil, blocked
	}

	if len(result.DisabledUserIDs) > 0 {
		c.kickUserLifecycleDelivery()
	}
	return result, nil
}

// CheckSCIMEnforceActor returns why the actor may not enforce the connection, or an empty problem when they may.
func (c *Client) CheckSCIMEnforceActor(ctx context.Context, conn *types.SCIMConnection, actor SCIMEnforceActor) (SCIMEnforceActorProblem, error) {
	return checkSCIMEnforceActorTx(c.db.WithContext(ctx), conn, actor)
}

// SCIMProvisionedUsers returns a page of the users that the connection has provisioned, oldest binding first, and
// how many there are.
func (c *Client) SCIMProvisionedUsers(ctx context.Context, conn *types.SCIMConnection, page SCIMPage) ([]SCIMSetupUser, int64, error) {
	db := c.db.WithContext(ctx)
	query := db.Table("scim_user_bindings").
		Joins("JOIN users ON users.id = scim_user_bindings.user_id").
		Where("scim_user_bindings.connection_id = ? AND scim_user_bindings.retired_at IS NULL", conn.ID)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count provisioned users: %w", err)
	}
	if page.Limit <= 0 || int64(page.Offset) >= total {
		return []SCIMSetupUser{}, total, nil
	}

	var rows []scimSetupUserRow
	if err := query.Select("users.*, scim_user_bindings.id AS scim_id, scim_user_bindings.active AS scim_active, ("+signedInSQL+") AS signed_in", conn.AuthProviderNamespace, conn.AuthProviderName).
		Order("scim_user_bindings.created_at, scim_user_bindings.id").
		Offset(page.Offset).
		Limit(page.Limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list provisioned users: %w", err)
	}

	users, err := c.scimSetupUsers(ctx, rows)
	return users, total, err
}

// SCIMUnprovisionedUsers returns a page of the live users of the connection's auth provider that the connection has
// not provisioned, oldest first, and how many there are. Enforcing SCIM disables them.
func (c *Client) SCIMUnprovisionedUsers(ctx context.Context, conn *types.SCIMConnection, page SCIMPage) ([]SCIMSetupUser, int64, error) {
	db := c.db.WithContext(ctx)

	var total int64
	if err := unprovisionedSCIMUsersQuery(db, conn).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count unprovisioned users: %w", err)
	}
	if page.Limit <= 0 || int64(page.Offset) >= total {
		return []SCIMSetupUser{}, total, nil
	}

	var rows []scimSetupUserRow
	if err := unprovisionedSCIMUsersQuery(db, conn).
		Select("users.*, ("+signedInSQL+") AS signed_in", conn.AuthProviderNamespace, conn.AuthProviderName).
		Order("users.id").
		Offset(page.Offset).
		Limit(page.Limit).
		Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list unprovisioned users: %w", err)
	}

	users, err := c.scimSetupUsers(ctx, rows)
	return users, total, err
}

// SCIMRequestFailurePage returns a page of the connection's recent failed requests, newest first, and how many are
// kept.
func (c *Client) SCIMRequestFailurePage(ctx context.Context, connectionID string, page SCIMPage) ([]types.SCIMRequestFailure, int64, error) {
	db := c.db.WithContext(ctx)

	var total int64
	if err := db.Model(new(types.SCIMRequestFailure)).Where("connection_id = ?", connectionID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count SCIM request failures: %w", err)
	}
	if page.Limit <= 0 || int64(page.Offset) >= total {
		return []types.SCIMRequestFailure{}, total, nil
	}

	var failures []types.SCIMRequestFailure
	if err := db.Where("connection_id = ?", connectionID).
		Order("id DESC").
		Offset(page.Offset).
		Limit(page.Limit).
		Find(&failures).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list SCIM request failures: %w", err)
	}
	for i := range failures {
		if err := c.decryptSCIMRequestFailure(ctx, &failures[i]); err != nil {
			return nil, 0, err
		}
	}
	return failures, total, nil
}

// WithNewSCIMGroupReferences runs write, which saves new references to groupIDs, once they are known to reach
// groups of a SCIM connection's auth provider, and returns what write returns. It returns a *SCIMGroupReferenceError
// without running write when a group ID has a connection's group ID prefix and no group of the connection's auth
// provider has it, or the group is being deleted because nothing referenced it. The identity provider pushes every
// group of such a provider, so a group that does not exist yet has no ID that a reference could use.
//
// The write is recorded before the references are checked, and until it finishes. A deletion of unreferenced groups
// whose marks commit after the check waits for it, and reads the references it saved; one whose marks commit before
// the check refuses it. No database connection is held while write runs. write must save the references before it
// returns, within scimReferenceWriteLifetime.
//
// Callers pass only the group IDs that a write adds, so an old reference to a missing group does not block unrelated
// edits.
func (c *Client) WithNewSCIMGroupReferences(ctx context.Context, groupIDs []string, write func() error) error {
	if len(groupIDs) == 0 {
		return write()
	}

	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return err
	}
	record := &types.SCIMReferenceWrite{
		ID:        uuid.NewV4().String(),
		ExpiresAt: now.Add(scimReferenceWriteLifetime),
	}
	if err := c.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("failed to record a write of group references: %w", err)
	}
	defer func() {
		// A record that is left behind only delays deletions until it expires.
		if err := c.db.WithContext(context.WithoutCancel(ctx)).Delete(record).Error; err != nil {
			slog.Warn("Failed to remove the record of a finished write of group references", "error", err)
		}
	}()

	if err := checkNewSCIMGroupReferencesTx(c.db.WithContext(ctx), groupIDs); err != nil {
		return err
	}
	return write()
}

// scimReferenceClock returns the time that writes of group references expire by. On PostgreSQL it is the database's
// clock, so that replicas whose clocks differ agree on when a write expires. SQLite runs a single replica, so the
// application's clock serves.
func (c *Client) scimReferenceClock(ctx context.Context) (time.Time, error) {
	db := c.db.WithContext(ctx)
	if db.Name() != "postgres" {
		return time.Now(), nil
	}

	var now time.Time
	if err := db.Raw("SELECT now()").Row().Scan(&now); err != nil {
		return time.Time{}, fmt.Errorf("failed to read the database clock: %w", err)
	}
	return now, nil
}

// WaitForSCIMReferenceWrites waits until the writes of group references recorded now have finished or expired. A
// write recorded later checks its groups after the caller's changes to them, such as marks for deletion, committed.
func (c *Client) WaitForSCIMReferenceWrites(ctx context.Context) error {
	db := c.db.WithContext(ctx)
	now, err := c.scimReferenceClock(ctx)
	if err != nil {
		return err
	}
	if err := db.Where("expires_at < ?", now).Delete(new(types.SCIMReferenceWrite)).Error; err != nil {
		return fmt.Errorf("failed to remove expired writes of group references: %w", err)
	}

	var ids []string
	if err := db.Model(new(types.SCIMReferenceWrite)).Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("failed to list writes of group references in progress: %w", err)
	}
	for len(ids) > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to wait for writes of group references: %w", ctx.Err())
		case <-time.After(scimReferenceWritePollInterval):
		}

		if now, err = c.scimReferenceClock(ctx); err != nil {
			return err
		}
		var remaining []string
		for batch := range slices.Chunk(ids, scimMemberBatchSize) {
			var found []string
			if err := db.Model(new(types.SCIMReferenceWrite)).
				Where("id IN ? AND expires_at >= ?", batch, now).
				Pluck("id", &found).Error; err != nil {
				return fmt.Errorf("failed to check writes of group references in progress: %w", err)
			}
			remaining = append(remaining, found...)
		}
		ids = remaining
	}
	return nil
}

func checkNewSCIMGroupReferencesTx(tx *gorm.DB, groupIDs []string) error {
	var conns []types.SCIMConnection
	if err := tx.Order("created_at, id").Find(&conns).Error; err != nil {
		return fmt.Errorf("failed to list SCIM connections: %w", err)
	}
	for _, conn := range conns {
		var candidates []string
		for _, id := range groupIDs {
			if conn.GroupIDPrefix != "" && strings.HasPrefix(id, conn.GroupIDPrefix) && !slices.Contains(candidates, id) {
				candidates = append(candidates, id)
			}
		}
		if len(candidates) == 0 {
			continue
		}

		existing := make(map[string]struct{}, len(candidates))
		pending := make(map[string]struct{}, len(candidates))
		for batch := range slices.Chunk(candidates, scimMemberBatchSize) {
			var ids []string
			if err := tx.Model(new(types.Group)).
				Where("auth_provider_namespace = ? AND auth_provider_name = ? AND id IN ?", conn.AuthProviderNamespace, conn.AuthProviderName, batch).
				Pluck("id", &ids).Error; err != nil {
				return fmt.Errorf("failed to check the groups of auth provider %s: %w", conn.AuthProviderName, err)
			}
			for _, id := range ids {
				existing[id] = struct{}{}
			}

			if err := tx.Model(new(types.SCIMPendingGroupDeletion)).
				Where("connection_id = ? AND group_id IN ?", conn.ID, batch).
				Pluck("group_id", &ids).Error; err != nil {
				return fmt.Errorf("failed to check the groups pending deletion of auth provider %s: %w", conn.AuthProviderName, err)
			}
			for _, id := range ids {
				pending[id] = struct{}{}
			}
		}

		refErr := &SCIMGroupReferenceError{
			AuthProviderNamespace: conn.AuthProviderNamespace,
			AuthProviderName:      conn.AuthProviderName,
		}
		for _, id := range candidates {
			if _, ok := existing[id]; !ok {
				refErr.Missing = append(refErr.Missing, id)
			} else if _, ok := pending[id]; ok {
				refErr.PendingDeletion = append(refErr.PendingDeletion, id)
			}
		}
		if len(refErr.Missing) > 0 || len(refErr.PendingDeletion) > 0 {
			return refErr
		}
	}
	return nil
}

func (c *Client) scimSetupUsers(ctx context.Context, rows []scimSetupUserRow) ([]SCIMSetupUser, error) {
	users := make([]SCIMSetupUser, 0, len(rows))
	for i := range rows {
		if err := c.decryptUser(ctx, &rows[i].User); err != nil {
			return nil, fmt.Errorf("failed to decrypt user %d: %w", rows[i].ID, err)
		}
		users = append(users, SCIMSetupUser{
			UserID:         rows[i].ID,
			Username:       rows[i].Username,
			Email:          rows[i].Email,
			DisplayName:    rows[i].DisplayName,
			DisabledAt:     rows[i].DisabledAt,
			DisabledReason: rows[i].DisabledReason,
			SCIMID:         rows[i].SCIMID,
			Active:         rows[i].Active,
			SignedIn:       rows[i].SignedIn,
		})
	}
	return users, nil
}

func authProviderGroupDataTx(tx *gorm.DB, namespace, name, groupIDPrefix string) (*AuthProviderGroupData, error) {
	groups, err := scimProviderGroupsTx(tx, namespace, name)
	if err != nil {
		return nil, err
	}
	data := &AuthProviderGroupData{
		Groups: groups,
	}
	if groupIDPrefix == "" {
		return data, nil
	}

	if err := tx.Model(new(types.GroupMemberships)).
		Where("substr(group_id, 1, ?) = ?", len(groupIDPrefix), groupIDPrefix).
		Count(&data.MembershipCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count the group memberships of auth provider %s/%s: %w", namespace, name, err)
	}
	if err := tx.Model(new(types.GroupRoleAssignment)).
		Where("substr(group_name, 1, ?) = ?", len(groupIDPrefix), groupIDPrefix).
		Order("group_name").
		Pluck("group_name", &data.RoleAssignmentGroupIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list the group role assignments of auth provider %s/%s: %w", namespace, name, err)
	}
	return data, nil
}

func scimProviderGroupsTx(tx *gorm.DB, namespace, name string) ([]SCIMProviderGroup, error) {
	var groups []SCIMProviderGroup
	if err := tx.Table("groups").
		Select("groups.id AS id, groups.name AS name, scim_group_bindings.id AS scim_id, scim_pending_group_deletions.group_id IS NOT NULL AS pending_deletion, COALESCE(scim_pending_group_deletions.run_id, '') AS pending_run_id").
		Joins("LEFT JOIN scim_group_bindings ON scim_group_bindings.group_id = groups.id AND scim_group_bindings.retired_at IS NULL").
		Joins("LEFT JOIN scim_pending_group_deletions ON scim_pending_group_deletions.group_id = groups.id").
		Where("groups.auth_provider_namespace = ? AND groups.auth_provider_name = ?", namespace, name).
		Order("groups.name, groups.id").
		Scan(&groups).Error; err != nil {
		return nil, fmt.Errorf("failed to list the groups of auth provider %s/%s: %w", namespace, name, err)
	}
	return groups, nil
}

// finishSCIMGroupDeletionTx finishes the deletion of unreferenced groups that MarkUnreferencedSCIMGroups started as
// the run runID, and returns the IDs of the groups it deleted. referenced holds the IDs of the groups that anything
// references, read after the marks were committed. The caller holds the SCIM write lock.
//
// A marked group that gained a reference in between is unmarked and kept, and so is one that SCIM bound or another
// deletion took over. The rest of the run's groups are deleted with their memberships, and the run's marks are
// cleared.
func finishSCIMGroupDeletionTx(tx *gorm.DB, conn *types.SCIMConnection, runID string, referenced map[string]struct{}) ([]string, error) {
	for batch := range slices.Chunk(slices.Sorted(maps.Keys(referenced)), scimMemberBatchSize) {
		if err := tx.Where("connection_id = ? AND group_id IN ?", conn.ID, batch).Delete(new(types.SCIMPendingGroupDeletion)).Error; err != nil {
			return nil, fmt.Errorf("failed to keep groups that gained a reference: %w", err)
		}
	}

	groups, err := scimProviderGroupsTx(tx, conn.AuthProviderNamespace, conn.AuthProviderName)
	if err != nil {
		return nil, err
	}
	var deleted []string
	for _, group := range groups {
		if !group.PendingDeletion || group.PendingRunID != runID || runID == "" || group.SCIMID != "" {
			continue
		}
		if _, ok := referenced[group.ID]; !ok {
			deleted = append(deleted, group.ID)
		}
	}
	if err := deleteGroupsTx(tx, conn, deleted); err != nil {
		return nil, err
	}
	return deleted, clearSCIMGroupDeletionMarksTx(tx, conn.ID, runID)
}

func clearSCIMGroupDeletionMarksTx(tx *gorm.DB, connectionID, runID string) error {
	if err := tx.Where("connection_id = ? AND run_id = ?", connectionID, runID).Delete(new(types.SCIMPendingGroupDeletion)).Error; err != nil {
		return fmt.Errorf("failed to clear the marks for deletion of SCIM connection %s: %w", connectionID, err)
	}
	return nil
}

func clearAllSCIMGroupDeletionMarksTx(tx *gorm.DB, connectionID string) error {
	if err := tx.Where("connection_id = ?", connectionID).Delete(new(types.SCIMPendingGroupDeletion)).Error; err != nil {
		return fmt.Errorf("failed to clear the marks for deletion of SCIM connection %s: %w", connectionID, err)
	}
	return nil
}

// deleteGroupsTx deletes groups of the connection's auth provider and their memberships.
func deleteGroupsTx(tx *gorm.DB, conn *types.SCIMConnection, groupIDs []string) error {
	for batch := range slices.Chunk(groupIDs, scimMemberBatchSize) {
		if err := tx.Where("group_id IN ?", batch).Delete(new(types.GroupMemberships)).Error; err != nil {
			return fmt.Errorf("failed to delete the memberships of unreferenced groups: %w", err)
		}
		if err := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ? AND id IN ?", conn.AuthProviderNamespace, conn.AuthProviderName, batch).
			Delete(new(types.Group)).Error; err != nil {
			return fmt.Errorf("failed to delete unreferenced groups: %w", err)
		}
	}
	return nil
}

// unprovisionedSCIMUsersQuery selects the live users with an identity of the connection's auth provider and no
// unretired binding in the connection. Users of other auth providers are never selected.
func unprovisionedSCIMUsersQuery(tx *gorm.DB, conn *types.SCIMConnection) *gorm.DB {
	return tx.Model(new(types.User)).
		Where("users.deleted_at IS NULL").
		Where("EXISTS (?)", tx.Model(new(types.Identity)).
			Select("1").
			Where("identities.user_id = users.id AND identities.auth_provider_namespace = ? AND identities.auth_provider_name = ?", conn.AuthProviderNamespace, conn.AuthProviderName)).
		Where("NOT EXISTS (?)", tx.Model(new(types.SCIMUserBinding)).
			Select("1").
			Where("scim_user_bindings.user_id = users.id AND scim_user_bindings.connection_id = ? AND scim_user_bindings.retired_at IS NULL", conn.ID))
}

// checkSCIMEnforceActorTx returns why the actor may not enforce the connection, or an empty problem when they may.
// Enforcing changes no memberships or roles, so a provisioned, active user who has signed in through the connection's
// auth provider keeps signing in with the role they have now.
func checkSCIMEnforceActorTx(tx *gorm.DB, conn *types.SCIMConnection, actor SCIMEnforceActor) (SCIMEnforceActorProblem, error) {
	if actor.UserID == 0 || actor.AuthProviderNamespace != conn.AuthProviderNamespace || actor.AuthProviderName != conn.AuthProviderName {
		return SCIMEnforceActorOtherAuthProvider, nil
	}

	var identities int64
	if err := tx.Model(new(types.Identity)).
		Where("user_id = ? AND auth_provider_namespace = ? AND auth_provider_name = ? AND first_sign_in_at IS NOT NULL", actor.UserID, conn.AuthProviderNamespace, conn.AuthProviderName).
		Count(&identities).Error; err != nil {
		return "", fmt.Errorf("failed to check the identities of user %d: %w", actor.UserID, err)
	}
	if identities == 0 {
		return SCIMEnforceActorNotSignedIn, nil
	}

	var bindings []types.SCIMUserBinding
	if err := tx.Select("id", "active").
		Where("connection_id = ? AND user_id = ? AND retired_at IS NULL", conn.ID, actor.UserID).
		Limit(1).
		Find(&bindings).Error; err != nil {
		return "", fmt.Errorf("failed to check the SCIM binding of user %d: %w", actor.UserID, err)
	}
	if len(bindings) == 0 {
		return SCIMEnforceActorUnprovisioned, nil
	}
	if !bindings[0].Active {
		return SCIMEnforceActorDeactivated, nil
	}

	if err := checkCredentialOwner(tx, actor.UserID); err != nil {
		if _, ok := errors.AsType[*UserAccessDeniedError](err); ok {
			return SCIMEnforceActorDisabled, nil
		}
		return "", err
	}
	var users int64
	if err := tx.Model(new(types.User)).Where("id = ? AND deleted_at IS NULL", actor.UserID).Count(&users).Error; err != nil {
		return "", fmt.Errorf("failed to check user %d: %w", actor.UserID, err)
	}
	if users == 0 {
		return SCIMEnforceActorDisabled, nil
	}
	return "", nil
}

// deleteSCIMConnectionTx deletes the connection, with all of its SCIM data and its auth provider's group data. The
// caller holds the SCIM mode and write locks, and the connection's row lock.
func deleteSCIMConnectionTx(tx *gorm.DB, conn *types.SCIMConnection) error {
	for _, model := range []any{new(types.SCIMUserBinding), new(types.SCIMGroupBinding), new(types.SCIMPendingGroupDeletion), new(types.SCIMRequestFailure)} {
		if err := tx.Where("connection_id = ?", conn.ID).Delete(model).Error; err != nil {
			return fmt.Errorf("failed to delete the data of SCIM connection %s: %w", conn.ID, err)
		}
	}
	// The auth provider cleanup removes every group with the prefix from access policies.
	if err := tx.Where("substr(group_id, 1, ?) = ?", len(conn.GroupIDPrefix), conn.GroupIDPrefix).
		Delete(new(types.SCIMGroupSubjectCleanup)).Error; err != nil {
		return fmt.Errorf("failed to delete the group subject cleanups of SCIM connection %s: %w", conn.ID, err)
	}
	if err := deleteAuthProviderGroupDataTx(tx, conn.AuthProviderNamespace, conn.AuthProviderName, conn.GroupIDPrefix); err != nil {
		return err
	}
	if err := tx.Delete(conn).Error; err != nil {
		return fmt.Errorf("failed to delete SCIM connection %s: %w", conn.ID, err)
	}
	return nil
}
