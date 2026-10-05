package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
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
	ID       string
	UserName string
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
}

// SCIMInvalidValueError reports a SCIM write whose values cannot be applied.
type SCIMInvalidValueError struct {
	Message string
}

// SCIMMutabilityError reports a SCIM write that changes an attribute that cannot change.
type SCIMMutabilityError struct {
	Message string
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
func (c *Client) ListSCIMUsers(ctx context.Context, connectionID string, filter SCIMUserFilter, page SCIMPage) ([]SCIMUser, int64, error) {
	var (
		total          int64
		users          []SCIMUser
		hashedUserName string
	)
	if filter.UserName != "" {
		var err error
		if hashedUserName, err = hashSCIMUserName(filter.UserName); err != nil {
			// No user has a userName that cannot be prepared.
			return nil, 0, nil
		}
	}

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(new(types.SCIMUserBinding)).Where("connection_id = ? AND retired_at IS NULL", connectionID)
		if filter.ID != "" {
			query = query.Where("id = ?", filter.ID)
		}
		if hashedUserName != "" {
			query = query.Where("hashed_user_name = ?", hashedUserName)
		}

		if err := query.Count(&total).Error; err != nil {
			return fmt.Errorf("failed to count SCIM users: %w", err)
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
				Message: fmt.Sprintf("a user with userName %q already exists", input.UserName),
			}
		}
		if err := tx.Model(new(types.SCIMUserBinding)).
			Where("connection_id = ? AND retired_at IS NULL AND hashed_native_user_id = ?", conn.ID, hashedNativeUserID).
			Count(&conflicts).Error; err != nil {
			return fmt.Errorf("failed to check SCIM externalId: %w", err)
		} else if conflicts > 0 {
			return &SCIMConflictError{
				Message: "a user with this externalId already exists",
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
				Message: "externalId identifies the user and cannot change",
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
						Message: fmt.Sprintf("a user with userName %q already exists", input.UserName),
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

// bindOrCreateSCIMUserTx returns the Obot user that a new SCIM user binds to, creating it when no live user has an
// identity of the connection's auth provider for the native user ID, and reports whether it was created.
func (c *Client) bindOrCreateSCIMUserTx(ctx context.Context, tx *gorm.DB, conn *types.SCIMConnection, a adapter.Adapter, nativeUserID, hashedNativeUserID string, input SCIMUserInput, opts SCIMUserCreateOptions) (uint, bool, error) {
	var identities []types.Identity
	if err := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", conn.AuthProviderNamespace, conn.AuthProviderName, hashedNativeUserID).
		Limit(1).
		Find(&identities).Error; err != nil {
		return 0, false, fmt.Errorf("failed to look up identity: %w", err)
	}

	var identity *types.Identity
	if len(identities) > 0 {
		identity = &identities[0]

		var users []types.User
		if err := tx.Select("id").Where("id = ? AND deleted_at IS NULL", identity.UserID).Limit(1).Find(&users).Error; err != nil {
			return 0, false, fmt.Errorf("failed to get user %d: %w", identity.UserID, err)
		}
		if len(users) > 0 {
			userID := users[0].ID

			// The native ID is unbound, but the user it belongs to may be bound under another one, which only an
			// identity provider that reassigned IDs could cause.
			if bound, err := activeSCIMUserBindingForUserTx(tx, userID, false); err != nil {
				return 0, false, err
			} else if bound != nil {
				return 0, false, &SCIMConflictError{
					Message: "the user with this externalId is already provisioned",
				}
			}

			if err := c.projectSCIMProfileTx(ctx, tx, userID, input.Profile); err != nil {
				return 0, false, err
			}
			return userID, false, nil
		}
		// The identity's user was deleted, so provisioning starts a new account. Nothing is inherited.
	}

	names := a.NewUserIdentity(nativeUserID)
	if names.Username == system.BootstrapName {
		return 0, false, &SCIMInvalidValueError{
			Message: fmt.Sprintf("%q is reserved and cannot identify a user", names.Username),
		}
	}

	// Roles are never set from SCIM, so an explicit role for the SCIM email is not granted here. Sign-in grants it for
	// that email.
	email := input.Profile.PrimaryEmail()
	verified := slices.Contains(verifiedAuthProviders, conn.AuthProviderNamespace+"/"+conn.AuthProviderName)

	// A new identity is inserted before the user, in the order a sign-in inserts them, so that a concurrent first
	// sign-in of the same person and this create wait for each other instead of deadlocking. Whichever inserts the
	// identity first wins; the other finds it, and a create that lost is retried to bind to the signed-in user.
	var newIdentity *types.Identity
	if identity == nil {
		newIdentity = &types.Identity{
			AuthProviderNamespace: conn.AuthProviderNamespace,
			AuthProviderName:      conn.AuthProviderName,
			ProviderUsername:      names.ProviderUsername,
			ProviderUserID:        names.ProviderUserID,
			HashedProviderUserID:  hashedNativeUserID,
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
					Message: "the user signed in while they were being provisioned; retry the request",
				},
			}
		}
	}

	user := &types.User{
		Username:       names.Username,
		HashedUsername: hash.String(names.Username),
		Email:          email,
		HashedEmail:    hashOptional(email),
		DisplayName:    input.Profile.ObotDisplayName(),
		VerifiedEmail:  &verified,
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
					Message: "an Obot user with this user's username already exists",
				},
			}
		}
		return 0, false, err
	}

	// A new identity is linked to the user, and an identity whose user was deleted moves to the new account, which
	// has not signed in yet.
	key := identity
	if key == nil {
		key = newIdentity
	}
	if err := tx.Model(new(types.Identity)).
		Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", key.AuthProviderNamespace, key.AuthProviderName, key.HashedProviderUserID).
		UpdateColumns(map[string]any{
			"user_id":          user.ID,
			"first_sign_in_at": nil,
		}).Error; err != nil {
		return 0, false, fmt.Errorf("failed to link identity to user %d: %w", user.ID, err)
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
		Select("email", "hashed_email", "display_name", "username", "icon_url", "original_email", "original_username", "encrypted").
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

// normalizeSCIMUserProfile makes equal profiles compare equal: an empty name is absent, and empty lists are nil.
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
