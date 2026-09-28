package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	userLifecycleDeliveryInterval  = 10 * time.Second
	userLifecycleDeliveryBatchSize = 100
	// userLifecycleClaimDuration is how long a replica holds an event it is delivering. A failed delivery keeps
	// its claim, so this is also the delay before the event is retried.
	userLifecycleClaimDuration = time.Minute
	// userLifecycleRetention is how long delivered events are kept for troubleshooting.
	userLifecycleRetention = 7 * 24 * time.Hour

	// AccountNotActiveMessage is what a user denied access by their lifecycle status is told. It does not reveal
	// the reason, which only administrators see.
	AccountNotActiveMessage = "Your account is not active. Contact your administrator."
)

// AuthProviderRef identifies an auth provider.
type AuthProviderRef struct {
	Namespace string
	Name      string
}

// UserAccessDeniedError reports that a user may not access Obot because they are disabled, deleted, or missing.
type UserAccessDeniedError struct {
	UserID uint
	Status types2.UserStatus
}

// UserAccessLookupError reports that a user's lifecycle status could not be determined. Callers must deny access,
// and must not fall back to another credential or to anonymous access.
type UserAccessLookupError struct {
	UserID uint
	Err    error
}

// UserDeletedError reports a lifecycle change for a deleted user. Deleted users stay deleted.
type UserDeletedError struct {
	UserID uint
}

// UserOutsideAuthProviderError reports a lifecycle change for a user with no identity for the auth provider making
// the change. Lifecycle changes never reach users of other auth providers.
type UserOutsideAuthProviderError struct {
	UserID   uint
	Provider AuthProviderRef
}

func (r AuthProviderRef) String() string {
	return r.Namespace + "/" + r.Name
}

func (e *UserAccessDeniedError) Error() string {
	return fmt.Sprintf("user %d is %s", e.UserID, e.Status)
}

// HTTPError returns the response for a request denied by this error.
func (e *UserAccessDeniedError) HTTPError() *types2.ErrHTTP {
	return types2.NewErrHTTP(http.StatusForbidden, AccountNotActiveMessage)
}

func (e *UserAccessLookupError) Error() string {
	if e.UserID == 0 {
		return fmt.Sprintf("failed to check the user's status: %v", e.Err)
	}
	return fmt.Sprintf("failed to check the status of user %d: %v", e.UserID, e.Err)
}

func (e *UserAccessLookupError) Unwrap() error {
	return e.Err
}

func (e *UserDeletedError) Error() string {
	return fmt.Sprintf("user %d is deleted", e.UserID)
}

func (e *UserOutsideAuthProviderError) Error() string {
	return fmt.Sprintf("user %d has no identity for auth provider %s", e.UserID, e.Provider)
}

// UserStatus reads a user's current lifecycle status. A user with no row is reported as deleted. It returns a
// *UserAccessLookupError if the status could not be read.
func (c *Client) UserStatus(ctx context.Context, userID uint) (types2.UserStatus, error) {
	status, _, err := userStatus(c.db.WithContext(ctx), userID)
	return status, err
}

// userStatus reads a user's current lifecycle status, and reports whether the user has a row. A user with no row is
// reported as deleted.
func userStatus(tx *gorm.DB, userID uint) (types2.UserStatus, bool, error) {
	var user types.User
	if err := tx.Select("id", "deleted_at", "disabled_at").Where("id = ?", userID).Take(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return types2.UserStatusDeleted, false, nil
	} else if err != nil {
		return "", false, &UserAccessLookupError{
			UserID: userID,
			Err:    err,
		}
	}
	return user.Status(), true, nil
}

// CheckCredentialOwner returns nil unless issuing a credential to the user must be refused. It reads the user's
// current state, and returns a *UserAccessDeniedError if the user is disabled or deleted, and a
// *UserAccessLookupError if the state could not be read. A user ID with no row is left to the admission check, which
// denies every use of a credential issued for it.
func (c *Client) CheckCredentialOwner(ctx context.Context, userID uint) error {
	return checkCredentialOwner(c.db.WithContext(ctx), userID)
}

func checkCredentialOwner(tx *gorm.DB, userID uint) error {
	status, found, err := userStatus(tx, userID)
	if err != nil || !found {
		return err
	}
	if status != types2.UserStatusActive {
		return &UserAccessDeniedError{
			UserID: userID,
			Status: status,
		}
	}
	return nil
}

// DisableUser denies a user access without deleting anything: the user keeps their account, identities, roles,
// memberships, API keys, and resources, and can be reactivated. Only a user with an identity for provider can be
// changed, and a deleted user cannot.
//
// In the same transaction, disabling deletes the user's gateway auth tokens and records an outbox event whose
// delivery deletes the user's MCP OAuth refresh tokens. Neither is restored on reactivation.
//
// Disabling a user who is already disabled only updates the reason, and records no event.
func (c *Client) DisableUser(ctx context.Context, provider AuthProviderRef, userID uint, reason types.UserDisabledReason) (*types.User, error) {
	var (
		user    *types.User
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, changed, err = disableUserTx(tx, provider, userID, reason)
		return err
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	return user, c.decryptUser(ctx, user)
}

// ReactivateUser restores access for a disabled user of provider, with the same ID and data. A deleted user cannot
// be reactivated. Reactivating a user who is not disabled changes nothing.
func (c *Client) ReactivateUser(ctx context.Context, provider AuthProviderRef, userID uint) (*types.User, error) {
	var user *types.User
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, _, err = reactivateUserTx(tx, provider, userID)
		return err
	}); err != nil {
		return nil, err
	}

	return user, c.decryptUser(ctx, user)
}

// disableUserTx disables a user within tx and reports whether the user lost access. The returned user is not
// decrypted. The caller must kick lifecycle delivery after committing when it reports a change.
func disableUserTx(tx *gorm.DB, provider AuthProviderRef, userID uint, reason types.UserDisabledReason) (*types.User, bool, error) {
	if !reason.Valid() {
		return nil, false, fmt.Errorf("invalid disable reason %q", reason)
	}

	user, err := lockLifecycleUser(tx, provider, userID)
	if err != nil {
		return nil, false, err
	}

	if user.DisabledAt != nil {
		if user.DisabledReason != reason {
			// Access is unchanged, so only the reason is updated, and there is nothing to revoke.
			if err := tx.Model(user).UpdateColumn("disabled_reason", reason).Error; err != nil {
				return nil, false, fmt.Errorf("failed to update the disable reason of user %d: %w", userID, err)
			}
			user.DisabledReason = reason
		}
		return user, false, nil
	}

	now := time.Now()
	if err := tx.Model(user).UpdateColumns(map[string]any{
		"disabled_at":     now,
		"disabled_reason": reason,
	}).Error; err != nil {
		return nil, false, fmt.Errorf("failed to disable user %d: %w", userID, err)
	}

	if err := tx.Where("user_id = ?", user.ID).Delete(new(types.AuthToken)).Error; err != nil {
		return nil, false, fmt.Errorf("failed to delete the auth tokens of user %d: %w", userID, err)
	}

	if err := tx.Create(&types.UserLifecycleEvent{
		UserID: user.ID,
		Type:   types.UserLifecycleEventDisabled,
	}).Error; err != nil {
		return nil, false, fmt.Errorf("failed to record the lifecycle event of user %d: %w", userID, err)
	}

	user.DisabledAt = &now
	user.DisabledReason = reason
	return user, true, nil
}

// reactivateUserTx reactivates a user within tx and reports whether the user regained access. The returned user is
// not decrypted.
func reactivateUserTx(tx *gorm.DB, provider AuthProviderRef, userID uint) (*types.User, bool, error) {
	user, err := lockLifecycleUser(tx, provider, userID)
	if err != nil {
		return nil, false, err
	}

	if user.DisabledAt == nil {
		return user, false, nil
	}

	// Explicit columns, because struct updates skip the zero values that mark a user as enabled.
	if err := tx.Model(user).UpdateColumns(map[string]any{
		"disabled_at":     nil,
		"disabled_reason": "",
	}).Error; err != nil {
		return nil, false, fmt.Errorf("failed to reactivate user %d: %w", userID, err)
	}

	user.DisabledAt = nil
	user.DisabledReason = ""
	return user, true, nil
}

// lockLifecycleUser loads and locks a user whose lifecycle is about to change, and verifies that the change is
// allowed: the user must not be deleted, and must have an identity for provider.
func lockLifecycleUser(tx *gorm.DB, provider AuthProviderRef, userID uint) (*types.User, error) {
	if provider.Namespace == "" || provider.Name == "" {
		return nil, errors.New("auth provider namespace and name are required")
	}

	user := new(types.User)
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(user).Error; err != nil {
		return nil, err
	}
	if user.DeletedAt != nil {
		return nil, &UserDeletedError{
			UserID: userID,
		}
	}

	var identities int64
	if err := tx.Model(new(types.Identity)).
		Where("user_id = ? AND auth_provider_namespace = ? AND auth_provider_name = ?", userID, provider.Namespace, provider.Name).
		Count(&identities).Error; err != nil {
		return nil, fmt.Errorf("failed to check the identities of user %d: %w", userID, err)
	}
	if identities == 0 {
		return nil, &UserOutsideAuthProviderError{
			UserID:   userID,
			Provider: provider,
		}
	}

	return user, nil
}

// recordUserReconcileEvent records, within tx, that a user's roles, and their group-granted resources if
// groupsRemoved is set, must be reconciled once tx commits. The caller must kick lifecycle delivery after
// committing.
func recordUserReconcileEvent(tx *gorm.DB, userID uint, groupsRemoved bool) error {
	if err := tx.Create(&types.UserLifecycleEvent{
		UserID:        userID,
		Type:          types.UserLifecycleEventReconcile,
		GroupsRemoved: groupsRemoved,
	}).Error; err != nil {
		return fmt.Errorf("failed to record the reconcile event of user %d: %w", userID, err)
	}
	return nil
}

// kickUserLifecycleDelivery asks the delivery loop to deliver new lifecycle events now instead of at its next tick.
// It never blocks.
func (c *Client) kickUserLifecycleDelivery() {
	select {
	case c.kickLifecycleDelivery <- struct{}{}:
	default:
	}
}

// runUserLifecycleEventDelivery delivers lifecycle events from the outbox until ctx is done. Every replica runs it.
// Claims keep replicas from usually delivering the same event, and delivery is idempotent when they do.
func (c *Client) runUserLifecycleEventDelivery(ctx context.Context) {
	if c.storageClient == nil {
		return
	}

	ticker := time.NewTicker(userLifecycleDeliveryInterval)
	defer ticker.Stop()

	for {
		if err := c.deliverUserLifecycleEvents(ctx); err != nil && ctx.Err() == nil {
			slog.Error("Failed to deliver user lifecycle events", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.kickLifecycleDelivery:
		}
	}
}

// deliverUserLifecycleEvents delivers pending outbox events, oldest first, and prunes old delivered events. A
// failed event is retried once its claim expires.
func (c *Client) deliverUserLifecycleEvents(ctx context.Context) error {
	now := time.Now()

	var events []types.UserLifecycleEvent
	if err := c.db.WithContext(ctx).
		Where("delivered_at IS NULL AND (claimed_until IS NULL OR claimed_until < ?)", now).
		Order("id").
		Limit(userLifecycleDeliveryBatchSize).
		Find(&events).Error; err != nil {
		return fmt.Errorf("failed to list user lifecycle events: %w", err)
	}

	for _, event := range events {
		claimed, err := c.claimUserLifecycleEvent(ctx, event.ID, now)
		if err != nil {
			return err
		}
		if !claimed {
			// Another replica is delivering it.
			continue
		}

		if err := c.deliverUserLifecycleEvent(ctx, event); err != nil {
			if updateErr := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
				Where("id = ?", event.ID).
				UpdateColumns(map[string]any{
					"attempts":   gorm.Expr("attempts + 1"),
					"last_error": err.Error(),
				}).Error; updateErr != nil {
				return errors.Join(err, updateErr)
			}
			slog.Warn("Failed to deliver user lifecycle event", "eventID", event.ID, "userID", event.UserID, "type", event.Type, "error", err)
			continue
		}

		if err := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
			Where("id = ?", event.ID).
			UpdateColumns(map[string]any{
				"delivered_at": time.Now(),
				"last_error":   "",
			}).Error; err != nil {
			return fmt.Errorf("failed to mark user lifecycle event %d delivered: %w", event.ID, err)
		}
	}

	if err := c.db.WithContext(ctx).
		Where("delivered_at IS NOT NULL AND delivered_at < ?", now.Add(-userLifecycleRetention)).
		Delete(new(types.UserLifecycleEvent)).Error; err != nil {
		return fmt.Errorf("failed to prune delivered user lifecycle events: %w", err)
	}

	return nil
}

// claimUserLifecycleEvent claims an undelivered event for this replica, and reports whether it did.
func (c *Client) claimUserLifecycleEvent(ctx context.Context, eventID uint, now time.Time) (bool, error) {
	result := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
		Where("id = ? AND delivered_at IS NULL AND (claimed_until IS NULL OR claimed_until < ?)", eventID, now).
		UpdateColumn("claimed_until", now.Add(userLifecycleClaimDuration))
	if result.Error != nil {
		return false, fmt.Errorf("failed to claim user lifecycle event %d: %w", eventID, result.Error)
	}
	return result.RowsAffected == 1, nil
}

func (c *Client) deliverUserLifecycleEvent(ctx context.Context, event types.UserLifecycleEvent) error {
	switch event.Type {
	case types.UserLifecycleEventDisabled:
		return c.deliverUserDisabledEvent(ctx, event)
	case types.UserLifecycleEventReconcile:
		return c.deliverUserReconcileEvent(ctx, event)
	default:
		return fmt.Errorf("unknown user lifecycle event type %q", event.Type)
	}
}

// deliverUserDisabledEvent ends the browser sessions and deletes the MCP OAuth refresh tokens of a user who is still
// denied access, so that neither works again if the user is reactivated. It reads the user's current state, so a
// late event for a reactivated user deletes nothing. A disabled user cannot sign in or obtain new refresh tokens, so a
// repeated delivery finds nothing left to delete.
func (c *Client) deliverUserDisabledEvent(ctx context.Context, event types.UserLifecycleEvent) error {
	if status, _, err := userStatus(c.db.WithContext(ctx), event.UserID); err != nil {
		return err
	} else if status == types2.UserStatusActive {
		return nil
	}

	// Revoking refresh tokens and ending sessions are independent, so a failure in one never keeps the other from
	// happening. The event is retried until both succeed.
	return errors.Join(c.deleteUserOAuthTokens(ctx, event.UserID), c.endUserSessions(ctx, event.UserID))
}

// deleteUserOAuthTokens deletes a user's MCP OAuth refresh tokens.
func (c *Client) deleteUserOAuthTokens(ctx context.Context, userID uint) error {
	var tokens v1.OAuthTokenList
	if err := c.storageClient.List(ctx, &tokens); err != nil {
		return fmt.Errorf("failed to list OAuth tokens: %w", err)
	}

	var errs []error
	for _, token := range tokens.Items {
		if token.Spec.UserID != userID {
			continue
		}
		if err := c.storageClient.Delete(ctx, &token); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("failed to delete an OAuth token of user %d: %w", userID, err))
			continue
		}
		slog.Info("Deleted OAuth refresh token of disabled user", "userID", userID, "clientID", token.Spec.ClientID)
	}

	return errors.Join(errs...)
}

// endUserSessions deletes a user's sessions from every session store the installation can delete them from.
func (c *Client) endUserSessions(ctx context.Context, userID uint) error {
	identities, err := c.FindIdentitiesForUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to list the identities of user %d: %w", userID, err)
	}
	if err := c.DeleteSessionsForUser(ctx, c.storageClient, identities, "", ""); err != nil && !sessionDeletionUnsupported(err) {
		return fmt.Errorf("failed to end the sessions of user %d: %w", userID, err)
	}
	return nil
}

// sessionDeletionUnsupported reports whether err only says that the installation cannot delete an auth provider's
// sessions. Without PostgreSQL, sessions of providers other than local auth live in cookies, which the admission
// check denies until they expire.
func sessionDeletionUnsupported(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, err := range joined.Unwrap() {
			if !sessionDeletionUnsupported(err) {
				return false
			}
		}
		return true
	}
	return errors.As(err, new(LogoutAllErr))
}

// deliverUserReconcileEvent creates the UserRoleChange, and the UserGroupChange if the user left a group, that make
// the controller reconcile what depends on the user's roles and groups. Their names come from the event, so a
// repeated delivery creates nothing new while they exist, and the handlers are idempotent if it does.
func (c *Client) deliverUserReconcileEvent(ctx context.Context, event types.UserLifecycleEvent) error {
	if err := c.storageClient.Create(ctx, &v1.UserRoleChange{
		Name:      userLifecycleObjectName(system.UserRoleChangePrefix, event.ID),
		Namespace: system.DefaultNamespace,
		Spec: v1.UserRoleChangeSpec{
			UserID: event.UserID,
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create the role change of user %d: %w", event.UserID, err)
	}

	if !event.GroupsRemoved {
		return nil
	}

	if err := c.storageClient.Create(ctx, &v1.UserGroupChange{
		Name:      userLifecycleObjectName(system.UserGroupChangePrefix, event.ID),
		Namespace: system.DefaultNamespace,
		Spec: v1.UserGroupChangeSpec{
			UserID: event.UserID,
		},
	}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create the group change of user %d: %w", event.UserID, err)
	}

	return nil
}

// userLifecycleObjectName names the controller object delivered for an outbox event. The name cannot collide with
// one generated from the same prefix, whose random suffix never contains a vowel.
func userLifecycleObjectName(prefix string, eventID uint) string {
	return fmt.Sprintf("%slifecycle-%d", prefix, eventID)
}
