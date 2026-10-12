package client

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
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

// userLifecycleDelivery delivers the events of one type for one user in a batch at once: a user in many changed
// groups needs one reconcile, not one per group.
type userLifecycleDelivery struct {
	// event is the first of the events. A reconcile event records that the user left a group if any of them does.
	event types.UserLifecycleEvent
	ids   []uint
}

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

// EnableUser restores the access of a disabled user, as an administrator decides to, with the same ID and data. It is
// refused with *SCIMManagedUserError while SCIM manages the user's access: while the user has a SCIM binding, or an
// identity of an auth provider that has a SCIM connection, whose identity provider decides whom it provisions. A
// deleted user cannot be enabled, and enabling a user who is not disabled changes nothing.
func (c *Client) EnableUser(ctx context.Context, userID uint) (*types.User, error) {
	user := new(types.User)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// No connection can be created, and SCIM cannot provision the user, while this transaction runs.
		if err := lockSCIMMode(tx, false); err != nil {
			return err
		}
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).First(user).Error; err != nil {
			return err
		}
		if user.DeletedAt != nil {
			return &UserDeletedError{
				UserID: userID,
			}
		}
		if user.DisabledAt == nil {
			return nil
		}

		binding, err := activeSCIMUserBindingForUserTx(tx, userID, false)
		if err != nil {
			return err
		}
		if binding != nil {
			return &SCIMManagedUserError{
				UserID:  userID,
				Message: "this user is managed by the identity provider through SCIM; reactivate them there",
			}
		}
		var managed int64
		if err := tx.Model(new(types.Identity)).
			Joins("JOIN scim_connections ON scim_connections.auth_provider_namespace = identities.auth_provider_namespace AND scim_connections.auth_provider_name = identities.auth_provider_name").
			Where("identities.user_id = ?", userID).
			Count(&managed).Error; err != nil {
			return fmt.Errorf("failed to check whether SCIM manages user %d: %w", userID, err)
		}
		if managed > 0 {
			return &SCIMManagedUserError{
				UserID:  userID,
				Message: "this user signs in through an auth provider that provisions users through SCIM; assign them in the identity provider instead",
			}
		}

		// Explicit columns, because struct updates skip the zero values that mark a user as enabled.
		if err := tx.Model(user).UpdateColumns(map[string]any{
			"disabled_at":     nil,
			"disabled_reason": "",
		}).Error; err != nil {
			return fmt.Errorf("failed to enable user %d: %w", userID, err)
		}
		user.DisabledAt = nil
		user.DisabledReason = ""
		return nil
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

// deliverUserLifecycleEvents delivers the pending outbox events, batch after batch, until a batch comes back short,
// then prunes old delivered events. A group change of thousands of members records as many events, and a kick only
// says that there is something to deliver, so the batches are delivered without waiting for the next tick.
func (c *Client) deliverUserLifecycleEvents(ctx context.Context) error {
	for {
		full, err := c.deliverUserLifecycleEventBatch(ctx)
		if err != nil {
			return err
		}
		if !full || ctx.Err() != nil {
			break
		}
	}

	if err := c.db.WithContext(ctx).
		Where("delivered_at IS NOT NULL AND delivered_at < ?", time.Now().Add(-userLifecycleRetention)).
		Delete(new(types.UserLifecycleEvent)).Error; err != nil {
		return fmt.Errorf("failed to prune delivered user lifecycle events: %w", err)
	}
	return nil
}

// deliverUserLifecycleEventBatch delivers one batch of pending outbox events, oldest first, and reports whether the
// batch was full, so more events may be pending. A failed event is retried once its claim expires.
func (c *Client) deliverUserLifecycleEventBatch(ctx context.Context) (bool, error) {
	now := time.Now()

	var ids []uint
	if err := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
		Where("delivered_at IS NULL AND (claimed_until IS NULL OR claimed_until < ?)", now).
		Order("id").
		Limit(userLifecycleDeliveryBatchSize).
		Pluck("id", &ids).Error; err != nil {
		return false, fmt.Errorf("failed to list user lifecycle events: %w", err)
	}
	if len(ids) == 0 {
		return false, nil
	}

	events, err := c.claimUserLifecycleEvents(ctx, ids, now)
	if err != nil {
		return false, err
	}

	// The OAuth tokens are listed once for the batch, and only if a disabled user needs theirs deleted.
	oauthTokens := c.listOAuthTokensOnce(ctx)

	var delivered []uint
	for _, delivery := range coalesceUserLifecycleEvents(events) {
		if err := c.deliverUserLifecycleEvent(ctx, delivery.event, oauthTokens); err != nil {
			if updateErr := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
				Where("id IN ?", delivery.ids).
				UpdateColumns(map[string]any{
					"attempts":   gorm.Expr("attempts + 1"),
					"last_error": err.Error(),
				}).Error; updateErr != nil {
				return false, errors.Join(err, updateErr)
			}
			slog.Warn("Failed to deliver user lifecycle event", "eventIDs", delivery.ids, "userID", delivery.event.UserID, "type", delivery.event.Type, "error", err)
			continue
		}
		delivered = append(delivered, delivery.ids...)
	}

	if len(delivered) > 0 {
		if err := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).
			Where("id IN ?", delivered).
			UpdateColumns(map[string]any{
				"delivered_at": time.Now(),
				"last_error":   "",
			}).Error; err != nil {
			return false, fmt.Errorf("failed to mark user lifecycle events delivered: %w", err)
		}
	}

	return len(ids) == userLifecycleDeliveryBatchSize, nil
}

// claimUserLifecycleEvents claims the undelivered events with ids for this replica in one statement, and returns
// them, oldest first. The events that another replica claimed in the meantime are left out.
func (c *Client) claimUserLifecycleEvents(ctx context.Context, ids []uint, now time.Time) ([]types.UserLifecycleEvent, error) {
	var events []types.UserLifecycleEvent
	if err := c.db.WithContext(ctx).Model(&events).Clauses(clause.Returning{}).
		Where("id IN ? AND delivered_at IS NULL AND (claimed_until IS NULL OR claimed_until < ?)", ids, now).
		UpdateColumn("claimed_until", now.Add(userLifecycleClaimDuration)).Error; err != nil {
		return nil, fmt.Errorf("failed to claim user lifecycle events: %w", err)
	}
	slices.SortFunc(events, func(a, b types.UserLifecycleEvent) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return events, nil
}

// listOAuthTokensOnce returns a function that lists every OAuth token the first time it is called, and returns the
// same answer every time after.
func (c *Client) listOAuthTokensOnce(ctx context.Context) func() ([]v1.OAuthToken, error) {
	return sync.OnceValues(func() ([]v1.OAuthToken, error) {
		var tokens v1.OAuthTokenList
		if err := c.storageClient.List(ctx, &tokens); err != nil {
			return nil, fmt.Errorf("failed to list OAuth tokens: %w", err)
		}
		return tokens.Items, nil
	})
}

// coalesceUserLifecycleEvents groups events, in the order of their IDs, into one delivery for each user and type.
func coalesceUserLifecycleEvents(events []types.UserLifecycleEvent) []userLifecycleDelivery {
	type key struct {
		userID    uint
		eventType types.UserLifecycleEventType
	}

	var (
		deliveries = make([]userLifecycleDelivery, 0, len(events))
		index      = make(map[key]int, len(events))
	)
	for _, event := range events {
		k := key{
			userID:    event.UserID,
			eventType: event.Type,
		}
		if i, ok := index[k]; ok {
			deliveries[i].ids = append(deliveries[i].ids, event.ID)
			deliveries[i].event.GroupsRemoved = deliveries[i].event.GroupsRemoved || event.GroupsRemoved
			continue
		}
		index[k] = len(deliveries)
		deliveries = append(deliveries, userLifecycleDelivery{
			event: event,
			ids:   []uint{event.ID},
		})
	}
	return deliveries
}

func (c *Client) deliverUserLifecycleEvent(ctx context.Context, event types.UserLifecycleEvent, oauthTokens func() ([]v1.OAuthToken, error)) error {
	switch event.Type {
	case types.UserLifecycleEventDisabled:
		return c.deliverUserDisabledEvent(ctx, event, oauthTokens)
	case types.UserLifecycleEventReconcile:
		return c.deliverUserReconcileEvent(ctx, event)
	default:
		return fmt.Errorf("unknown user lifecycle event type %q", event.Type)
	}
}

// deliverUserDisabledEvent ends the browser sessions and deletes the MCP OAuth refresh tokens of a user who is still
// denied access, so that neither works again if the user is reactivated. It reads the user's current state, so a
// late event for a reactivated user deletes nothing. A disabled user cannot sign in or obtain new refresh tokens, so a
// repeated delivery finds nothing left to delete. oauthTokens lists every OAuth token.
func (c *Client) deliverUserDisabledEvent(ctx context.Context, event types.UserLifecycleEvent, oauthTokens func() ([]v1.OAuthToken, error)) error {
	if status, _, err := userStatus(c.db.WithContext(ctx), event.UserID); err != nil {
		return err
	} else if status == types2.UserStatusActive {
		return nil
	}

	// Revoking refresh tokens and ending sessions are independent, so a failure in one never keeps the other from
	// happening. The event is retried until both succeed.
	return errors.Join(c.deleteUserOAuthTokens(ctx, event.UserID, oauthTokens), c.endUserSessions(ctx, event.UserID))
}

// deleteUserOAuthTokens deletes a user's MCP OAuth refresh tokens. oauthTokens lists every OAuth token: they can
// only be listed all at once, so a batch of deliveries lists them once.
func (c *Client) deleteUserOAuthTokens(ctx context.Context, userID uint, oauthTokens func() ([]v1.OAuthToken, error)) error {
	tokens, err := oauthTokens()
	if err != nil {
		return err
	}

	var errs []error
	for _, token := range tokens {
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
