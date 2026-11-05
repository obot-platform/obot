package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5/pgconn"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apiserver/pkg/storage/value"
)

const (
	// scimUserCreateAttempts bounds the attempts of a SCIM user create. A create is retried once when a concurrent
	// sign-in creates the identity or user it was about to create, so that the retry binds to the signed-in user.
	scimUserCreateAttempts = 2

	// postgresDeadlockDetected and postgresSerializationFailure are the SQLSTATE codes of transactions that
	// PostgreSQL aborted because of a concurrent one.
	postgresDeadlockDetected     = "40P01"
	postgresSerializationFailure = "40001"
)

// SCIMUserInput holds the writable attributes of a SCIM user, as a create or a full replacement sends them.
type SCIMUserInput struct {
	UserName   string
	ExternalID string
	// Active is the provisioned state. Nil means active on a create, and keeps the current state on a replacement.
	Active  *bool
	Profile types.SCIMUserProfile
}

// SCIMUser is a SCIM user as the SCIM endpoint serves it.
type SCIMUser struct {
	ID         string
	UserID     uint
	UserName   string
	ExternalID string
	Active     bool
	Profile    types.SCIMUserProfile
	// Groups are the bound groups the user is a direct member of.
	Groups    []SCIMGroupReference
	Revision  int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SCIMGroupReference names a bound group.
type SCIMGroupReference struct {
	ID          string
	DisplayName string
}

// SCIMUserFilter selects SCIM users. Empty fields match everything.
type SCIMUserFilter struct {
	ID         string
	UserName   string
	ExternalID string
}

// SCIMPage selects a page of results. Offset is zero-based. A zero Limit returns only the total.
type SCIMPage struct {
	Offset int
	Limit  int
}

// SCIMUserCreateOptions holds what a SCIM user create needs from outside the gateway database.
type SCIMUserCreateOptions struct {
	UserLimit UserLimit
	// DefaultRole is the role of users that SCIM creates.
	DefaultRole types2.Role
}

// SCIMNotFoundError reports a SCIM resource that does not exist in the connection. Retired resources and resources
// that are not bound are reported the same way.
type SCIMNotFoundError struct {
	ResourceType types.SCIMResourceType
	ID           string
}

// SCIMConflictError reports a SCIM write that conflicts with an existing resource.
type SCIMConflictError struct {
	Message string
	// Uniqueness is set for a conflict with attribute values that are already in use, which is what RFC 7644's
	// uniqueness error means. Without it, such as for a conflict with the state of Obot, the response has no scimType.
	Uniqueness bool
}

// SCIMInvalidValueError reports a SCIM write whose values cannot be applied.
type SCIMInvalidValueError struct {
	Message string
}

// SCIMMutabilityError reports a SCIM write that changes an attribute that cannot change.
type SCIMMutabilityError struct {
	// Resource and Attribute name the SCIM attribute that cannot change, so that the SCIM endpoint can explain the
	// change.
	Resource  types.SCIMResourceType
	Attribute string
	Message   string
}

// SCIMManagedUserError reports an Obot-side change that is refused because SCIM manages the user.
type SCIMManagedUserError struct {
	UserID  uint
	Message string
}

// scimCreateRaceError reports a SCIM user create that collided with a concurrent sign-in of the same person. The
// create is retried, and err is returned if the retry collides again.
type scimCreateRaceError struct {
	err error
}

func (e *SCIMNotFoundError) Error() string {
	return fmt.Sprintf("%s %s not found", e.ResourceType, e.ID)
}

func (e *SCIMConflictError) Error() string {
	return e.Message
}

func (e *SCIMInvalidValueError) Error() string {
	return e.Message
}

func (e *SCIMMutabilityError) Error() string {
	return e.Message
}

func (e *SCIMManagedUserError) Error() string {
	return e.Message
}

func (e *scimCreateRaceError) Error() string {
	return e.err.Error()
}

func (e *scimCreateRaceError) Unwrap() error {
	return e.err
}

// ListSCIMUsers returns the connection's bound users that match filter, oldest first, and the total number of matches.
// Users that SCIM has not provisioned are never returned.
func (c *Client) ListSCIMUsers(ctx context.Context, conn *types.SCIMConnection, filter SCIMUserFilter, page SCIMPage) ([]SCIMUser, int64, error) {
	var (
		total              int64
		users              []SCIMUser
		hashedUserName     string
		hashedNativeUserID string
		connectionID       = conn.ID
	)
	if filter.UserName != "" {
		var err error
		if hashedUserName, err = hashSCIMUserName(filter.UserName); err != nil {
			// No user has a userName that cannot be prepared.
			return nil, 0, nil
		}
	}
	if filter.ExternalID != "" {
		a, err := connectionAdapter(conn)
		if err != nil {
			return nil, 0, err
		}
		// A bound user's native ID is the one the adapter derives from their externalId.
		nativeUserID, err := a.NativeUserIDFromExternalID(filter.ExternalID)
		if err != nil {
			// No user has an externalId that the adapter refuses.
			return nil, 0, nil
		}
		hashedNativeUserID = hash.String(nativeUserID)
	}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(new(types.SCIMUserBinding)).Where("connection_id = ? AND retired_at IS NULL", connectionID)
		if filter.ID != "" {
			query = query.Where("id = ?", filter.ID)
		}
		if hashedUserName != "" {
			query = query.Where("hashed_user_name = ?", hashedUserName)
		}
		if hashedNativeUserID != "" {
			query = query.Where("hashed_native_user_id = ?", hashedNativeUserID)
		}

		if err := query.Count(&total).Error; err != nil {
			return fmt.Errorf("failed to count SCIM users: %w", err)
		}
		if hashedNativeUserID != "" && total > 0 {
			// The native user ID identifies at most one user. The adapter can derive it from externalIds other than
			// the user's, such as one with surrounding space, but externalId is case exact, so the user matches only
			// when theirs is the one filtered for.
			var binding types.SCIMUserBinding
			if err := query.Take(&binding).Error; err != nil {
				return fmt.Errorf("failed to get SCIM user: %w", err)
			}
			if err := c.decryptSCIMUserBinding(ctx, &binding); err != nil {
				return err
			}
			if binding.ExternalID != filter.ExternalID {
				total = 0
			}
		}
		if page.Limit <= 0 || int64(page.Offset) >= total {
			return nil
		}

		var bindings []types.SCIMUserBinding
		if err := query.Order("created_at, id").Offset(page.Offset).Limit(page.Limit).Find(&bindings).Error; err != nil {
			return fmt.Errorf("failed to list SCIM users: %w", err)
		}

		userIDs := make([]uint, 0, len(bindings))
		for _, binding := range bindings {
			userIDs = append(userIDs, binding.UserID)
		}
		groups, err := scimUserGroupsTx(tx, connectionID, userIDs)
		if err != nil {
			return err
		}

		users = make([]SCIMUser, 0, len(bindings))
		for i := range bindings {
			if err := c.decryptSCIMUserBinding(ctx, &bindings[i]); err != nil {
				return err
			}
			user, err := scimUserFromBinding(&bindings[i], groups[bindings[i].UserID])
			if err != nil {
				return err
			}
			users = append(users, *user)
		}
		return nil
	}); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

// GetSCIMUser returns the connection's bound user with the given SCIM ID.
func (c *Client) GetSCIMUser(ctx context.Context, connectionID, id string) (*SCIMUser, error) {
	var user *SCIMUser
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = c.scimUserTx(ctx, tx, connectionID, id)
		return err
	}); err != nil {
		return nil, err
	}
	return user, nil
}

// CreateSCIMUser provisions a SCIM user. It binds the user to the existing user whose identity for the connection's
// exact auth provider stores the same native user ID, and otherwise creates a user and identity through the
// seat-checked path. Email is never evidence of identity.
//
// A user that was deleted in Obot is never restored: provisioning the same person again creates a new, empty account
// and relinks the identity to it.
func (c *Client) CreateSCIMUser(ctx context.Context, conn *types.SCIMConnection, input SCIMUserInput, opts SCIMUserCreateOptions) (*SCIMUser, error) {
	a, err := connectionAdapter(conn)
	if err != nil {
		return nil, err
	}

	nativeUserID, err := a.NativeUserID(adapter.User{
		UserName:   input.UserName,
		ExternalID: input.ExternalID,
	})
	if err != nil {
		return nil, scimAdapterError(err)
	}
	if err := validateSCIMUserInput(input); err != nil {
		return nil, err
	}

	for attempt := 1; ; attempt++ {
		user, err := c.createSCIMUser(ctx, conn, a, nativeUserID, input, opts)
		if race, ok := errors.AsType[*scimCreateRaceError](err); ok {
			if attempt < scimUserCreateAttempts {
				continue
			}
			return nil, race.err
		}
		if isTransactionConflict(err) && attempt < scimUserCreateAttempts {
			// PostgreSQL aborted the transaction to break a deadlock or a serialization conflict with a concurrent
			// writer, which a retry resolves.
			continue
		}
		return user, err
	}
}

func (c *Client) createSCIMUser(ctx context.Context, conn *types.SCIMConnection, a adapter.Adapter, nativeUserID string, input SCIMUserInput, opts SCIMUserCreateOptions) (*SCIMUser, error) {
	var (
		user    *SCIMUser
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if conn, err = lockSCIMConnectionWrites(tx, conn.ID); err != nil {
			return err
		}

		hashedUserName, err := hashSCIMUserName(input.UserName)
		if err != nil {
			return err
		}
		hashedNativeUserID := hash.String(nativeUserID)

		var conflicts int64
		if err := tx.Model(new(types.SCIMUserBinding)).
			Where("connection_id = ? AND retired_at IS NULL AND hashed_user_name = ?", conn.ID, hashedUserName).
			Count(&conflicts).Error; err != nil {
			return fmt.Errorf("failed to check SCIM userName: %w", err)
		} else if conflicts > 0 {
			return &SCIMConflictError{
				Message:    fmt.Sprintf("a user with userName %q already exists", input.UserName),
				Uniqueness: true,
			}
		}
		if err := tx.Model(new(types.SCIMUserBinding)).
			Where("connection_id = ? AND retired_at IS NULL AND hashed_native_user_id = ?", conn.ID, hashedNativeUserID).
			Count(&conflicts).Error; err != nil {
			return fmt.Errorf("failed to check SCIM externalId: %w", err)
		} else if conflicts > 0 {
			return &SCIMConflictError{
				Message:    "a user with this externalId already exists",
				Uniqueness: true,
			}
		}

		userID, created, err := c.bindOrCreateSCIMUserTx(ctx, tx, conn, a, nativeUserID, hashedNativeUserID, input, opts)
		if err != nil {
			return err
		}

		active := input.Active == nil || *input.Active
		binding := &types.SCIMUserBinding{
			ID:                 uuid.NewV4().String(),
			ConnectionID:       conn.ID,
			UserID:             userID,
			HashedNativeUserID: hashedNativeUserID,
			HashedUserName:     hashedUserName,
			Active:             active,
			Revision:           1,
		}
		if err := setSCIMUserBindingAttributes(binding, input); err != nil {
			return err
		}

		stored := *binding
		if err := c.encryptSCIMUserBinding(ctx, &stored); err != nil {
			return err
		}
		if err := tx.Create(&stored).Error; err != nil {
			return fmt.Errorf("failed to create SCIM user binding: %w", err)
		}
		binding.CreatedAt = stored.CreatedAt
		binding.UpdatedAt = stored.UpdatedAt

		if changed, err = syncSCIMUserLifecycleTx(tx, conn, userID, active); err != nil {
			return err
		}

		if created {
			// A user created outside a sign-in still needs its role reconciled, which a sign-in would do.
			if err := recordUserReconcileEvent(tx, userID, false); err != nil {
				return err
			}
			changed = true
		}

		user, err = scimUserFromBinding(binding, nil)
		return err
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	return user, nil
}

// UpdateSCIMUser changes a bound user. The replacement is computed by mutate from the user's current state, inside the
// transaction that writes it, so that concurrent writes cannot interleave. A replacement identical to the current state
// changes nothing.
func (c *Client) UpdateSCIMUser(ctx context.Context, conn *types.SCIMConnection, id string, mutate func(current SCIMUser) (SCIMUserInput, error)) (*SCIMUser, error) {
	a, err := connectionAdapter(conn)
	if err != nil {
		return nil, err
	}

	var (
		user    *SCIMUser
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSCIMWrites(tx); err != nil {
			return err
		}

		binding, err := activeSCIMUserBindingTx(tx, conn.ID, id, true)
		if err != nil {
			return err
		}
		if err := c.decryptSCIMUserBinding(ctx, binding); err != nil {
			return err
		}
		// The user's groups are read-only here, so the current ones are also those of the response. mutate gets them,
		// so that a PATCH can restate them unchanged.
		groups, err := scimUserGroupsTx(tx, conn.ID, []uint{binding.UserID})
		if err != nil {
			return err
		}
		current, err := scimUserFromBinding(binding, groups[binding.UserID])
		if err != nil {
			return err
		}

		input, err := mutate(*current)
		if err != nil {
			return err
		}
		if input.ExternalID == "" {
			// The externalId is the identity evidence the user was bound by, so omitting it keeps it.
			input.ExternalID = current.ExternalID
		}
		if err := validateSCIMUserInput(input); err != nil {
			return err
		}

		nativeUserID, err := a.NativeUserID(adapter.User{
			UserName:   input.UserName,
			ExternalID: input.ExternalID,
		})
		if err != nil {
			return scimAdapterError(err)
		}
		if hash.String(nativeUserID) != binding.HashedNativeUserID {
			return &SCIMMutabilityError{
				Resource:  types.SCIMResourceTypeUser,
				Attribute: "externalId",
				Message:   "externalId identifies the user and cannot change",
			}
		}

		active := current.Active
		if input.Active != nil {
			active = *input.Active
		}

		profileChanged := !reflect.DeepEqual(normalizeSCIMUserProfile(current.Profile), normalizeSCIMUserProfile(input.Profile))
		if current.UserName != input.UserName || current.ExternalID != input.ExternalID || current.Active != active || profileChanged {
			hashedUserName, err := hashSCIMUserName(input.UserName)
			if err != nil {
				return err
			}
			if hashedUserName != binding.HashedUserName {
				var conflicts int64
				if err := tx.Model(new(types.SCIMUserBinding)).
					Where("connection_id = ? AND retired_at IS NULL AND hashed_user_name = ? AND id != ?", conn.ID, hashedUserName, binding.ID).
					Count(&conflicts).Error; err != nil {
					return fmt.Errorf("failed to check SCIM userName: %w", err)
				} else if conflicts > 0 {
					return &SCIMConflictError{
						Message:    fmt.Sprintf("a user with userName %q already exists", input.UserName),
						Uniqueness: true,
					}
				}
			}

			updated := *binding
			updated.HashedUserName = hashedUserName
			updated.Active = active
			updated.Revision++
			updated.UpdatedAt = time.Now()
			if err := setSCIMUserBindingAttributes(&updated, input); err != nil {
				return err
			}

			stored := updated
			if err := c.encryptSCIMUserBinding(ctx, &stored); err != nil {
				return err
			}
			// Explicit columns, so that false and empty values are written too.
			if err := tx.Model(&stored).
				Select("hashed_user_name", "external_id", "user_name", "profile", "encrypted", "active", "revision", "updated_at").
				Updates(&stored).Error; err != nil {
				return fmt.Errorf("failed to update SCIM user binding: %w", err)
			}

			if profileChanged {
				if err := c.projectSCIMProfileTx(ctx, tx, binding.UserID, input.Profile); err != nil {
					return err
				}
			}
			binding = &updated
		}

		// The lifecycle is reconciled even when the binding is unchanged, so that the user's access always follows the
		// provisioned state.
		if changed, err = syncSCIMUserLifecycleTx(tx, conn, binding.UserID, active); err != nil {
			return err
		}

		user, err = scimUserFromBinding(binding, groups[binding.UserID])
		return err
	}); err != nil {
		return nil, err
	}

	if changed {
		c.kickUserLifecycleDelivery()
	}

	return user, nil
}

// DeleteSCIMUser deletes a bound user, as DELETE /Users/{id} asks of an identity provider whose adapter's Rules
// delete users, and returns the user's ID. In one transaction, the user is disabled, which ends their access when it
// commits, the binding is retired, and the user is deleted as an administrator deletes them. The identity provider is
// the one deleting the user, so unlike the deletion by an administrator, an active binding does not refuse it.
//
// A retired or unknown SCIM ID is a *SCIMNotFoundError. Deleting Obot's last Owner, or its last Admin when there is no
// other Owner, is refused with a *SCIMConflictError, which the identity provider retries.
func (c *Client) DeleteSCIMUser(ctx context.Context, conn *types.SCIMConnection, id string) (uint, error) {
	var userID uint
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if conn, err = lockSCIMConnectionWrites(tx, conn.ID); err != nil {
			return err
		}

		binding, err := activeSCIMUserBindingTx(tx, conn.ID, id, true)
		if err != nil {
			return err
		}
		userID = binding.UserID
		if err := refuseClaudeTagUserMutation(tx, strconv.FormatUint(uint64(userID), 10)); err != nil {
			return err
		}

		// Disabling deletes the user's gateway tokens in this transaction, and records the removal of their MCP OAuth
		// tokens, so that their access ends at commit rather than when the cleanup runs. A user who is already
		// disabled stays disabled.
		user, _, err := disableUserTx(tx, AuthProviderRef{
			Namespace: conn.AuthProviderNamespace,
			Name:      conn.AuthProviderName,
		}, userID, types.UserDisabledReasonSCIMDeleted)
		if err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(binding).UpdateColumns(map[string]any{
			"retired_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("failed to retire the SCIM binding of user %d: %w", userID, err)
		}

		err = refuseLastOwnerOrAdminTx(tx, user)
		if err == nil {
			err = c.deleteUserTx(ctx, tx, user)
		}
		if _, ok := errors.AsType[*LastOwnerError](err); ok {
			return &SCIMConflictError{
				Message: "this user is Obot's last owner; make another user an owner in Obot before deleting them in the identity provider",
			}
		}
		if _, ok := errors.AsType[*LastAdminError](err); ok {
			return &SCIMConflictError{
				Message: "this user is Obot's last admin; make another user an admin in Obot before deleting them in the identity provider",
			}
		}
		return err
	}); err != nil {
		return 0, err
	}

	c.kickUserLifecycleDelivery()
	return userID, nil
}

// bindOrCreateSCIMUserTx returns the Obot user that a new SCIM user binds to, creating it when no live user has an
// identity of the connection's auth provider for the native user ID, and reports whether it was created. The
// adapter's UserKey selects which identity column the native ID matches.
//
// Matching by group lookup ID can find more than one identity, such as a duplicate that sign-in created before SCIM
// knew the person. The live user that SCIM has not bound is preferred. More than one such user is refused rather than
// guessed between.
func (c *Client) bindOrCreateSCIMUserTx(ctx context.Context, tx *gorm.DB, conn *types.SCIMConnection, a adapter.Adapter, nativeUserID, hashedNativeUserID string, input SCIMUserInput, opts SCIMUserCreateOptions) (uint, bool, error) {
	query := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ?", conn.AuthProviderNamespace, conn.AuthProviderName)
	if a.UserKey() == adapter.UserKeyGroupLookupID {
		// A first sign-in of the same person takes the same lock before it creates or claims an identity, so that
		// either this finds the identity it created, or it claims the identity this creates.
		if err := lockSCIMPersonTx(tx, conn.AuthProviderNamespace, conn.AuthProviderName, nativeUserID); err != nil {
			return 0, false, err
		}
		query = query.Where("hashed_provider_group_lookup_id = ?", hashedNativeUserID).Order("hashed_provider_user_id")
	} else {
		query = query.Where("hashed_provider_user_id = ?", hashedNativeUserID).Limit(1)
	}
	var identities []types.Identity
	if err := query.Find(&identities).Error; err != nil {
		return 0, false, fmt.Errorf("failed to look up identity: %w", err)
	}

	userIDs := make([]uint, 0, len(identities))
	for _, identity := range identities {
		if identity.UserID != 0 && !slices.Contains(userIDs, identity.UserID) {
			userIDs = append(userIDs, identity.UserID)
		}
	}
	var liveUserIDs []uint
	if len(userIDs) > 0 {
		if err := tx.Model(new(types.User)).Where("id IN ? AND deleted_at IS NULL", userIDs).Order("id").Pluck("id", &liveUserIDs).Error; err != nil {
			return 0, false, fmt.Errorf("failed to get the users of the identities: %w", err)
		}
	}

	if len(liveUserIDs) > 0 {
		unbound := make([]uint, 0, len(liveUserIDs))
		for _, userID := range liveUserIDs {
			// The native ID is unbound, but the user it belongs to may be bound under another one, which only an
			// identity provider that reassigned IDs could cause.
			if bound, err := activeSCIMUserBindingForUserTx(tx, userID, false); err != nil {
				return 0, false, err
			} else if bound == nil {
				unbound = append(unbound, userID)
			}
		}
		switch len(unbound) {
		case 0:
			return 0, false, &SCIMConflictError{
				Message:    "the user with this externalId is already provisioned",
				Uniqueness: true,
			}
		case 1:
			if err := c.projectSCIMProfileTx(ctx, tx, unbound[0], input.Profile); err != nil {
				return 0, false, err
			}
			return unbound[0], false, nil
		default:
			return 0, false, &SCIMConflictError{
				Message:    fmt.Sprintf("%d Obot users have the identity provider ID in this externalId; delete the duplicates in Obot", len(unbound)),
				Uniqueness: true,
			}
		}
	}
	// No live user has an identity for the native ID, so provisioning starts a new account. The identities of deleted
	// users, if any, move to it, and nothing else is inherited.

	names := a.NewUserIdentity(nativeUserID)
	if a.UserKey() == adapter.UserKeyGroupLookupID {
		// An identity that a sign-in created keeps the subject it was signed in with, so the new account gets the
		// username that sign-in gives, as it would if the person signed in first. With several such identities, the
		// first by key sets it. A username that another user has fails the create below as a conflict.
		pendingKey := hash.String(adapter.PendingProviderUserID(nativeUserID))
		for i := range identities {
			if identities[i].HashedProviderUserID == pendingKey {
				continue
			}
			identity := identities[i]
			if err := c.decryptStoredIdentity(ctx, &identity); err != nil {
				return 0, false, err
			}
			names.Username = identity.ProviderUsername
			break
		}
	}
	if names.Username == system.BootstrapName {
		return 0, false, &SCIMInvalidValueError{
			Message: fmt.Sprintf("%q is reserved and cannot identify a user", names.Username),
		}
	}

	email := input.Profile.PrimaryEmail()

	// A new identity is inserted before the user, in the order a sign-in inserts them, so that a concurrent first
	// sign-in of the same person and this create wait for each other instead of deadlocking. Whichever inserts the
	// identity first wins; the other finds it, and a create that lost is retried to bind to the signed-in user.
	if len(identities) == 0 {
		newIdentity := &types.Identity{
			AuthProviderNamespace: conn.AuthProviderNamespace,
			AuthProviderName:      conn.AuthProviderName,
			ProviderUsername:      names.ProviderUsername,
			ProviderUserID:        names.ProviderUserID,
			HashedProviderUserID:  hash.String(names.ProviderUserID),
			ProviderGroupLookupID: names.ProviderGroupLookupID,
			Email:                 email,
			HashedEmail:           hashOptional(email),
		}
		if err := c.encryptIdentity(ctx, newIdentity); err != nil {
			return 0, false, fmt.Errorf("failed to encrypt identity: %w", err)
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(newIdentity)
		if result.Error != nil {
			return 0, false, fmt.Errorf("failed to create identity: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return 0, false, &scimCreateRaceError{
				err: &SCIMConflictError{
					Message:    "the user signed in while they were being provisioned; retry the request",
					Uniqueness: true,
				},
			}
		}
		identities = append(identities, *newIdentity)
	}

	user := &types.User{
		Username:       names.Username,
		HashedUsername: hash.String(names.Username),
		Email:          email,
		HashedEmail:    hashOptional(email),
		DisplayName:    input.Profile.ObotDisplayName(),
		Role:           opts.DefaultRole,
	}
	if err := c.encryptUser(ctx, user); err != nil {
		return 0, false, fmt.Errorf("failed to encrypt user: %w", err)
	}
	if err := c.createUser(tx, user, opts.UserLimit); err != nil {
		if IsUniqueViolation(err) {
			// A concurrent sign-in may have just created the user, which the retry binds to.
			return 0, false, &scimCreateRaceError{
				err: &SCIMConflictError{
					Message:    "an Obot user with this user's username already exists",
					Uniqueness: true,
				},
			}
		}
		return 0, false, err
	}

	// A new identity is linked to the user, and identities whose user was deleted move to the new account, which has
	// not signed in yet.
	for _, key := range identities {
		if err := tx.Model(new(types.Identity)).
			Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", key.AuthProviderNamespace, key.AuthProviderName, key.HashedProviderUserID).
			UpdateColumns(map[string]any{
				"user_id":          user.ID,
				"first_sign_in_at": nil,
			}).Error; err != nil {
			return 0, false, fmt.Errorf("failed to link identity to user %d: %w", user.ID, err)
		}
	}

	return user.ID, true, nil
}

// syncSCIMUserLifecycleTx makes the user's access follow the provisioned state: an inactive user is disabled, and an
// active user is re-enabled, including one disabled for never having been provisioned. It reports whether the user's
// access changed, in which case the caller must kick lifecycle delivery after committing.
func syncSCIMUserLifecycleTx(tx *gorm.DB, conn *types.SCIMConnection, userID uint, active bool) (bool, error) {
	provider := AuthProviderRef{
		Namespace: conn.AuthProviderNamespace,
		Name:      conn.AuthProviderName,
	}

	var (
		changed bool
		err     error
	)
	if active {
		_, changed, err = reactivateUserTx(tx, provider, userID)
	} else {
		_, changed, err = disableUserTx(tx, provider, userID, types.UserDisabledReasonSCIMInactive)
	}
	return changed, err
}

// projectSCIMProfileTx copies the SCIM profile attributes that Obot's User represents onto the user: the primary email
// and the display name. Empty values leave the user's current values in place.
func (c *Client) projectSCIMProfileTx(ctx context.Context, tx *gorm.DB, userID uint, profile types.SCIMUserProfile) error {
	email := profile.PrimaryEmail()
	displayName := profile.ObotDisplayName()
	if email == "" && displayName == "" {
		return nil
	}

	// The user is locked before it is read, so that a concurrent sign-in or profile refresh either writes first, and
	// is overwritten here, or waits and then finds the user provisioned.
	user := new(types.User)
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", userID).Take(user).Error; err != nil {
		return fmt.Errorf("failed to get user %d: %w", userID, err)
	}
	if err := c.decryptUser(ctx, user); err != nil {
		return fmt.Errorf("failed to decrypt user: %w", err)
	}

	changed := false
	if email != "" && email != user.Email {
		user.Email = email
		user.HashedEmail = hash.String(email)
		changed = true
	}
	if displayName != "" && displayName != user.DisplayName {
		user.DisplayName = displayName
		changed = true
	}
	if !changed {
		return nil
	}

	if err := c.encryptUser(ctx, user); err != nil {
		return fmt.Errorf("failed to encrypt user: %w", err)
	}
	// Every encrypted column is written, so that the row stays consistent with its encrypted flag.
	if err := tx.Model(user).
		Select(slices.Concat(userEncryptedColumns, []string{"hashed_email"})).
		Updates(user).Error; err != nil {
		return fmt.Errorf("failed to update the profile of user %d: %w", userID, err)
	}
	return nil
}

func (c *Client) scimUserTx(ctx context.Context, tx *gorm.DB, connectionID, id string) (*SCIMUser, error) {
	binding, err := activeSCIMUserBindingTx(tx, connectionID, id, false)
	if err != nil {
		return nil, err
	}
	if err := c.decryptSCIMUserBinding(ctx, binding); err != nil {
		return nil, err
	}

	groups, err := scimUserGroupsTx(tx, connectionID, []uint{binding.UserID})
	if err != nil {
		return nil, err
	}
	return scimUserFromBinding(binding, groups[binding.UserID])
}

// scimUserFromBinding converts a decrypted binding.
func scimUserFromBinding(binding *types.SCIMUserBinding, groups []SCIMGroupReference) (*SCIMUser, error) {
	var profile types.SCIMUserProfile
	if binding.Profile != "" {
		if err := json.Unmarshal([]byte(binding.Profile), &profile); err != nil {
			return nil, fmt.Errorf("failed to decode the SCIM profile of user %s: %w", binding.ID, err)
		}
	}

	return &SCIMUser{
		ID:         binding.ID,
		UserID:     binding.UserID,
		UserName:   binding.UserName,
		ExternalID: binding.ExternalID,
		Active:     binding.Active,
		Profile:    profile,
		Groups:     groups,
		Revision:   binding.Revision,
		CreatedAt:  binding.CreatedAt,
		UpdatedAt:  binding.UpdatedAt,
	}, nil
}

// activeSCIMUserBindingTx returns the connection's unretired user binding with the given SCIM ID, locking it when
// forUpdate is set. The returned binding is still encrypted.
func activeSCIMUserBindingTx(tx *gorm.DB, connectionID, id string, forUpdate bool) (*types.SCIMUserBinding, error) {
	query := tx
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var bindings []types.SCIMUserBinding
	if err := query.Where("id = ? AND connection_id = ? AND retired_at IS NULL", id, connectionID).Limit(1).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("failed to get SCIM user %s: %w", id, err)
	}
	if len(bindings) == 0 {
		return nil, &SCIMNotFoundError{
			ResourceType: types.SCIMResourceTypeUser,
			ID:           id,
		}
	}
	return &bindings[0], nil
}

// scimUserGroupsTx returns the bound groups of the connection that each user is a direct member of.
func scimUserGroupsTx(tx *gorm.DB, connectionID string, userIDs []uint) (map[uint][]SCIMGroupReference, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	var rows []struct {
		UserID uint
		ID     string
		Name   string
	}
	if err := tx.Table("group_memberships").
		Select("group_memberships.user_id AS user_id, scim_group_bindings.id AS id, groups.name AS name").
		Joins("JOIN scim_group_bindings ON scim_group_bindings.group_id = group_memberships.group_id AND scim_group_bindings.retired_at IS NULL AND scim_group_bindings.connection_id = ?", connectionID).
		Joins("JOIN groups ON groups.id = group_memberships.group_id").
		Where("group_memberships.user_id IN ?", userIDs).
		Order("groups.name, scim_group_bindings.id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list the SCIM groups of users: %w", err)
	}

	groups := make(map[uint][]SCIMGroupReference, len(userIDs))
	for _, row := range rows {
		groups[row.UserID] = append(groups[row.UserID], SCIMGroupReference{
			ID:          row.ID,
			DisplayName: row.Name,
		})
	}
	return groups, nil
}

// SCIMUserBindingForUser returns the unretired SCIM binding of a user, or nil when SCIM has not provisioned the user.
// The returned binding is still encrypted.
func (c *Client) SCIMUserBindingForUser(ctx context.Context, userID uint) (*types.SCIMUserBinding, error) {
	return activeSCIMUserBindingForUserTx(c.db.WithContext(ctx), userID, false)
}

// SCIMManagedUsers returns the users in userIDs with an unretired SCIM binding.
func (c *Client) SCIMManagedUsers(ctx context.Context, userIDs []uint) (map[uint]struct{}, error) {
	provisioned := make(map[uint]struct{}, len(userIDs))
	for batch := range slices.Chunk(userIDs, scimMemberBatchSize) {
		var ids []uint
		if err := c.db.WithContext(ctx).Model(new(types.SCIMUserBinding)).
			Where("user_id IN ? AND retired_at IS NULL", batch).
			Pluck("user_id", &ids).Error; err != nil {
			return nil, fmt.Errorf("failed to list SCIM-provisioned users: %w", err)
		}
		for _, id := range ids {
			provisioned[id] = struct{}{}
		}
	}
	return provisioned, nil
}

func activeSCIMUserBindingForUserTx(tx *gorm.DB, userID uint, forUpdate bool) (*types.SCIMUserBinding, error) {
	query := tx
	if forUpdate {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var bindings []types.SCIMUserBinding
	if err := query.Where("user_id = ? AND retired_at IS NULL", userID).Limit(1).Find(&bindings).Error; err != nil {
		return nil, fmt.Errorf("failed to get the SCIM binding of user %d: %w", userID, err)
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	return &bindings[0], nil
}

// retireSCIMUserBindingForDeletionTx retires the SCIM binding of a user who is being deleted, so that its SCIM ID
// answers 404 and userName lookups no longer find the user. Only a user whose identity provider has deprovisioned
// them can be deleted: the identity provider would otherwise keep sending updates to a SCIM ID that no longer
// exists. A user disabled for never having been provisioned has no binding, and neither has a user SCIM never
// managed.
func retireSCIMUserBindingForDeletionTx(tx *gorm.DB, userID uint) error {
	// The lock is taken even when the user has no binding, so that a concurrent SCIM create cannot bind the user while
	// it is being deleted.
	if err := lockSCIMWrites(tx); err != nil {
		return err
	}
	binding, err := activeSCIMUserBindingForUserTx(tx, userID, true)
	if err != nil || binding == nil {
		return err
	}
	if binding.Active {
		return &SCIMManagedUserError{
			UserID:  userID,
			Message: "this user is still active in the identity provider; remove their assignment there before deleting them in Obot",
		}
	}

	now := time.Now()
	if err := tx.Model(binding).UpdateColumns(map[string]any{
		"retired_at": now,
		"updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("failed to retire the SCIM binding of user %d: %w", userID, err)
	}
	return nil
}

// refuseSCIMProvisionedUserTx returns a *SCIMManagedUserError with message when SCIM has provisioned the user.
func refuseSCIMProvisionedUserTx(tx *gorm.DB, userID uint, message string) error {
	binding, err := activeSCIMUserBindingForUserTx(tx, userID, false)
	if err != nil {
		return err
	}
	if binding != nil {
		return &SCIMManagedUserError{
			UserID:  userID,
			Message: message,
		}
	}
	return nil
}

func setSCIMUserBindingAttributes(binding *types.SCIMUserBinding, input SCIMUserInput) error {
	profile, err := json.Marshal(normalizeSCIMUserProfile(input.Profile))
	if err != nil {
		return fmt.Errorf("failed to encode SCIM profile: %w", err)
	}

	binding.ExternalID = input.ExternalID
	binding.UserName = input.UserName
	binding.Profile = string(profile)
	return nil
}

func validateSCIMUserInput(input SCIMUserInput) error {
	if strings.TrimSpace(input.UserName) == "" {
		return &SCIMInvalidValueError{
			Message: "userName is required",
		}
	}
	_, err := hashSCIMUserName(input.UserName)
	return err
}

// normalizeSCIMUserProfile makes equal profiles compare equal: an empty name, enterprise extension, or manager is
// absent, and empty lists are nil.
func normalizeSCIMUserProfile(profile types.SCIMUserProfile) types.SCIMUserProfile {
	if profile.Name != nil && *profile.Name == (types.SCIMName{}) {
		profile.Name = nil
	}
	if len(profile.Emails) == 0 {
		profile.Emails = nil
	}
	if len(profile.PhoneNumbers) == 0 {
		profile.PhoneNumbers = nil
	}
	if len(profile.Addresses) == 0 {
		profile.Addresses = nil
	}
	if profile.EnterpriseUser != nil {
		enterprise := *profile.EnterpriseUser
		if enterprise.Manager != nil && *enterprise.Manager == (types.SCIMManager{}) {
			enterprise.Manager = nil
		}
		if enterprise == (types.SCIMEnterpriseUser{}) {
			profile.EnterpriseUser = nil
		} else {
			profile.EnterpriseUser = &enterprise
		}
	}
	return profile
}

// connectionAdapter returns the adapter that the connection's persisted type selects.
func connectionAdapter(conn *types.SCIMConnection) (adapter.Adapter, error) {
	a, ok := adapter.Lookup(conn.AdapterType)
	if !ok {
		return nil, fmt.Errorf("SCIM connection %s has unknown adapter type %q", conn.ID, conn.AdapterType)
	}
	return a, nil
}

func scimAdapterError(err error) error {
	if invalid, ok := errors.AsType[*adapter.InvalidUserError](err); ok {
		return &SCIMInvalidValueError{
			Message: invalid.Message,
		}
	}
	if invalid, ok := errors.AsType[*adapter.InvalidGroupError](err); ok {
		return &SCIMInvalidValueError{
			Message: invalid.Message,
		}
	}
	return err
}

// hashSCIMUserName returns the hash of the key that userName is compared by, or a *SCIMInvalidValueError for a
// userName that has no key.
func hashSCIMUserName(userName string) (string, error) {
	key, err := types.SCIMUserNameKey(userName)
	if err != nil {
		return "", &SCIMInvalidValueError{
			Message: fmt.Sprintf("userName %q has characters that a username cannot have", userName),
		}
	}
	return hash.String(key), nil
}

func hashOptional(s string) string {
	if s == "" {
		return ""
	}
	return hash.String(s)
}

// isTransactionConflict reports whether PostgreSQL aborted a transaction because it deadlocked or conflicted with a
// concurrent one. Retrying the transaction is safe.
func isTransactionConflict(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && (pgErr.Code == postgresDeadlockDetected || pgErr.Code == postgresSerializationFailure)
}

func (c *Client) encryptSCIMUserBinding(ctx context.Context, binding *types.SCIMUserBinding) error {
	if c.encryptionConfig == nil {
		return nil
	}
	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := scimUserBindingDataCtx(binding.ID)
	for _, field := range []*string{&binding.ExternalID, &binding.UserName, &binding.Profile} {
		b, err := transformer.TransformToStorage(ctx, []byte(*field), dataCtx)
		if err != nil {
			return fmt.Errorf("failed to encrypt SCIM user binding: %w", err)
		}
		*field = base64.StdEncoding.EncodeToString(b)
	}
	binding.Encrypted = true
	return nil
}

func (c *Client) decryptSCIMUserBinding(ctx context.Context, binding *types.SCIMUserBinding) error {
	if !binding.Encrypted || c.encryptionConfig == nil {
		return nil
	}
	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := scimUserBindingDataCtx(binding.ID)
	for _, field := range []*string{&binding.ExternalID, &binding.UserName, &binding.Profile} {
		out, err := decryptSCIMUserBindingValue(ctx, transformer, dataCtx, *field)
		if err != nil {
			return err
		}
		*field = out
	}
	binding.Encrypted = false
	return nil
}

// decryptSCIMUserBindingField decrypts one encrypted field of the SCIM user binding with the given ID.
func (c *Client) decryptSCIMUserBindingField(ctx context.Context, bindingID, field string) (string, error) {
	if c.encryptionConfig == nil {
		return field, nil
	}
	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return field, nil
	}
	return decryptSCIMUserBindingValue(ctx, transformer, scimUserBindingDataCtx(bindingID), field)
}

func decryptSCIMUserBindingValue(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(field)
	if err != nil {
		return "", fmt.Errorf("failed to decode SCIM user binding: %w", err)
	}
	out, _, err := transformer.TransformFromStorage(ctx, decoded, dataCtx)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt SCIM user binding: %w", err)
	}
	return string(out), nil
}

func scimUserBindingDataCtx(bindingID string) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/scim/%s", userGroupResource.String(), bindingID))
}
