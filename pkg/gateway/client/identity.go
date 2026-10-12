package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	userCreationAdvisoryLockID int64 = 0x6f626f7455736572 // "obotUser"
)

var (
	verifiedAuthProviders = []string{
		"default/google-auth-provider",
		"default/github-auth-provider",
	}

	identityGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "identities",
	}
)

// ensuredIdentity is what ensureIdentity found or created.
type ensuredIdentity struct {
	user *types.User
	// created means ensureIdentity created the user.
	created bool
	// roleRaised means ensureIdentity raised an existing user's stored role, and recorded a reconcile event.
	roleRaised bool
	// scimManaged means a SCIM connection manages the identity's auth provider, so the provider is never asked for
	// groups.
	scimManaged bool
}

// scimSignInMode is the SCIM mode of a sign-in's auth provider: whether a SCIM connection manages it, and whether
// the connection is enforced.
type scimSignInMode struct {
	namespace, name string

	connectionID string
	enforced     bool
	locked       bool
}

// signInUserRow is a user that a sign-in matched, with whether SCIM provisioned them: through any connection, and
// through the connection of the sign-in's auth provider.
type signInUserRow struct {
	types.User
	SCIMBound             bool
	SCIMBoundToConnection bool
}

// FindIdentitiesForUser finds all identities for the given user.
func (c *Client) FindIdentitiesForUser(ctx context.Context, userID uint) ([]types.Identity, error) {
	var identities []types.Identity
	if err := c.db.WithContext(ctx).Where("user_id = ?", userID).Find(&identities).Error; err != nil {
		return nil, err
	}

	for i := range identities {
		if err := c.decryptIdentity(ctx, &identities[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt identity: %w", err)
		}
	}

	return identities, nil
}

// HasSignedInOwner reports whether a user other than the bootstrap user counts as an Owner of the named auth
// provider: their stored role includes Owner, they are not deleted, and they have signed in through the provider. An
// identity alone is not enough, because an identity can exist before its user has signed in. A disabled Owner still
// counts: the identity provider that disabled them can reactivate them, while the bootstrap token was printed to the
// server logs.
func (c *Client) HasSignedInOwner(ctx context.Context, authProviderName string) (bool, error) {
	var owners []types.User
	if err := c.db.WithContext(ctx).
		Where("deleted_at IS NULL AND (role & ?) != 0 AND hashed_username != ?", types2.RoleOwner, hash.String(system.BootstrapName)).
		Where("EXISTS (SELECT 1 FROM identities WHERE identities.user_id = users.id AND identities.auth_provider_name = ? AND identities.first_sign_in_at IS NOT NULL)", authProviderName).
		Find(&owners).Error; err != nil {
		return false, err
	}

	for i := range owners {
		if err := c.decryptUser(ctx, &owners[i]); err != nil {
			return false, err
		}
		// Users without an email are not people who signed in.
		if owners[i].Email != "" {
			return true, nil
		}
	}

	return false, nil
}

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
// The user gets the explicit role of their email, which ensureIdentityUser applies.
func (c *Client) EnsureIdentity(ctx context.Context, id *types.Identity, timezone string, userLimit UserLimit) (*types.User, error) {
	return c.EnsureIdentityWithRole(ctx, id, timezone, types2.RoleUnknown, userLimit)
}

// EnsureIdentityWithRole ensures the given identity exists in the database with the at least the given role, and returns the user associated with it.
// If the user already exists with a superset of the given role, it will not be updated.
func (c *Client) EnsureIdentityWithRole(ctx context.Context, id *types.Identity, timezone string, role types2.Role, userLimit UserLimit) (*types.User, error) {
	var ensured ensuredIdentity

	// Transaction #1: ensure the identity + user rows exist / are corrected, and read what we need.
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		ensured, err = c.ensureIdentity(ctx, tx, id, timezone, role, userLimit)
		return err
	})
	if err != nil {
		return nil, err
	}
	user, created := ensured.user, ensured.created
	if ensured.roleRaised {
		c.kickUserLifecycleDelivery()
	}

	if user.DisabledAt != nil {
		// A disabled user is denied by the admission check, which reads their state from the returned user. Don't
		// contact the auth provider or change anything else on their behalf.
		return user, nil
	}

	if ensured.scimManaged {
		// SCIM owns this provider's groups and memberships, so the auth provider is never asked for them.
		groups, err := c.listCachedGroups(ctx, *id)
		if err != nil {
			return nil, err
		}
		id.AuthProviderGroups = groups
	} else if err := c.ensureIdentityProviderData(ctx, id); err != nil {
		// Fetch and persist auth-provider data (group lookup ID and group memberships).
		// This makes HTTP calls to the auth provider and MUST run outside of any open DB
		// transaction so that we don't hold a pooled DB connection across network round-trips
		// (which can deadlock in-process auth providers that share the single SQLite connection).
		return nil, err
	}

	userRoleChanged := created || user.Role == types2.RoleUnknown
	if user.Role == types2.RoleUnknown {
		user.Role, err = c.getDefaultRole(ctx)
		if err != nil {
			return nil, err
		}

		if user, err = c.UpdateUser(ctx, true, user, fmt.Sprintf("%d", user.ID)); err != nil {
			return nil, err
		}
	}

	if userRoleChanged {
		if err = c.createUserRoleChangeForNewUser(ctx, user); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// EncryptIdentities will pull all identities out of the database and ensure they are encrypted.
func (c *Client) EncryptIdentities(ctx context.Context, force bool) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identities []types.Identity
		if err := tx.Find(&identities).Error; err != nil {
			return err
		}

		for i := range identities {
			if !force && identities[i].Encrypted {
				continue
			}

			if err := c.decryptIdentity(ctx, &identities[i]); err != nil {
				return fmt.Errorf("failed to decrypt identity: %w", err)
			}

			if err := c.encryptIdentity(ctx, &identities[i]); err != nil {
				return fmt.Errorf("failed to encrypt identity: %w", err)
			}

			// Omit the group check column to prevent resetting the group refresh window
			if err := tx.Omit(groupsLastCheckedColumn).Updates(identities[i]).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// ensureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
// It also reports whether it created the user, whether it raised an existing user's stored role, and whether SCIM
// manages the identity's auth provider. A raised role records a reconcile event in tx, so the caller must kick
// lifecycle delivery after committing.
//
// Once SCIM is enforced for the provider, sign-in requires a SCIM binding, and neither the identity nor the user is
// ever created here. Before that, users are still created just in time. Either way, sign-in never overwrites the
// profile of a user that SCIM has provisioned.
func (c *Client) ensureIdentity(ctx context.Context, tx *gorm.DB, id *types.Identity, timezone string, role types2.Role, userLimit UserLimit) (ensuredIdentity, error) {
	verified := slices.Contains(verifiedAuthProviders, fmt.Sprintf("%s/%s", id.AuthProviderNamespace, id.AuthProviderName))

	email := id.Email
	providerUserID := id.ProviderUserID
	providerUsername := id.ProviderUsername

	if id.ProviderUserID != "" {
		id.HashedProviderUserID = hash.String(id.ProviderUserID)
	}
	if id.Email != "" {
		id.HashedEmail = hash.String(id.Email)
	}

	// The SCIM mode is read with the identity, without a lock, which suffices for a sign-in that creates nothing. One
	// that creates an identity or a user reads it again under the shared mode lock first. A failed lookup fails the
	// sign-in, and never falls back to the directory.
	mode := &scimSignInMode{
		namespace: id.AuthProviderNamespace,
		name:      id.AuthProviderName,
	}
	user, created, roleRaised, err := c.ensureIdentityUser(ctx, tx, mode, id, timezone, role, userLimit, verified, email, providerUserID, providerUsername)
	if err != nil {
		return ensuredIdentity{}, err
	}
	return ensuredIdentity{
		user:        user,
		created:     created,
		roleRaised:  roleRaised,
		scimManaged: mode.connectionID != "",
	}, nil
}

func (m *scimSignInMode) set(connectionID string, state types.SCIMConnectionState) {
	m.connectionID = connectionID
	m.enforced = connectionID != "" && state == types.SCIMConnectionStateEnforced
}

// lock takes the SCIM mode lock shared and reads the mode again, for a sign-in that is about to create an identity or
// a user. The mode then holds until the transaction ends.
func (m *scimSignInMode) lock(tx *gorm.DB) error {
	if m.locked {
		return nil
	}
	conn, err := scimConnectionForAuthProviderLockedTx(tx, m.namespace, m.name)
	if err != nil {
		return err
	}
	if conn == nil {
		m.set("", "")
	} else {
		m.set(conn.ID, conn.State)
	}
	m.locked = true
	return nil
}

// identityWithSCIMModeTx reads the identity that id's key names into id, and the SCIM mode of its auth provider into
// mode, in one query. It reports whether the identity exists. When it does not, the mode is not read.
func identityWithSCIMModeTx(tx *gorm.DB, id *types.Identity, mode *scimSignInMode) (bool, error) {
	var rows []struct {
		types.Identity
		SCIMConnectionID    *string
		SCIMConnectionState *types.SCIMConnectionState
	}
	if err := tx.Model(new(types.Identity)).
		Select("identities.*, scim_connections.id AS scim_connection_id, scim_connections.state AS scim_connection_state").
		Joins("LEFT JOIN scim_connections ON scim_connections.auth_provider_namespace = identities.auth_provider_namespace AND scim_connections.auth_provider_name = identities.auth_provider_name").
		Where("identities.auth_provider_name = ? AND identities.auth_provider_namespace = ? AND identities.hashed_provider_user_id = ?", id.AuthProviderName, id.AuthProviderNamespace, id.HashedProviderUserID).
		Limit(1).
		Scan(&rows).Error; err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}

	groups := id.AuthProviderGroups
	*id = rows[0].Identity
	id.AuthProviderGroups = groups
	if rows[0].SCIMConnectionID != nil && rows[0].SCIMConnectionState != nil {
		mode.set(*rows[0].SCIMConnectionID, *rows[0].SCIMConnectionState)
	}
	return true, nil
}

// scimUnprovisionedSignInError refuses a sign-in that SCIM has not provisioned while SCIM is enforced. The user, if
// there is one, is reported as disabled, as Enforce disables the users it finds unprovisioned.
func scimUnprovisionedSignInError(userID uint) error {
	return &UserAccessDeniedError{
		UserID: userID,
		Status: types2.UserStatusDisabled,
	}
}

// ensureIdentityUser does the work of ensureIdentity. When SCIM is enforced for the identity's auth provider, sign-in
// requires an identity whose live user the connection provisioned, and it never creates an identity or a user.
func (c *Client) ensureIdentityUser(ctx context.Context, tx *gorm.DB, mode *scimSignInMode, id *types.Identity, timezone string, role types2.Role, userLimit UserLimit, verified bool, email, providerUserID, providerUsername string) (*types.User, bool, bool, error) {
	// See if the identity already exists.
	if found, err := identityWithSCIMModeTx(tx, id, mode); err != nil {
		return nil, false, false, fmt.Errorf("failed to look up identity: %w", err)
	} else if !found {
		// A first sign-in creates the identity, so the mode must hold until the transaction ends.
		if err := mode.lock(tx); err != nil {
			return nil, false, false, err
		}
		if mode.enforced {
			return nil, false, false, scimUnprovisionedSignInError(0)
		}

		// The identity does not exist.
		// Before we try creating a new identity, we need to check if there is one that has not been fully migrated yet.
		migratedIdentity := &types.Identity{
			ProviderUsername:      id.ProviderUsername,
			HashedProviderUserID:  hash.String(fmt.Sprintf("OBOT_PLACEHOLDER_%s", id.ProviderUsername)),
			AuthProviderName:      id.AuthProviderName,
			AuthProviderNamespace: id.AuthProviderNamespace,
		}
		if err = tx.First(migratedIdentity).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			// The identity does not exist, so create it.
			if err = c.encryptIdentity(ctx, id); err != nil {
				return nil, false, false, fmt.Errorf("failed to encrypt identity: %w", err)
			}
			// A concurrent request may win this insert on a first sign-in.
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(id).Error; err != nil {
				return nil, false, false, err
			}
			// Read back whichever row won, so both racers continue with the same user ID.
			if err = tx.Where(
				"auth_provider_name = ? AND auth_provider_namespace = ? AND hashed_provider_user_id = ?",
				id.AuthProviderName, id.AuthProviderNamespace, id.HashedProviderUserID,
			).First(id).Error; err != nil {
				return nil, false, false, err
			}
		} else if err != nil {
			return nil, false, false, err
		} else {
			if err = c.encryptIdentity(ctx, id); err != nil {
				return nil, false, false, fmt.Errorf("failed to encrypt identity: %w", err)
			}

			// The migrated identity exists. We need to update it with the right provider_user_id.
			if err = tx.Model(&migratedIdentity).Where("hashed_provider_user_id = ?", migratedIdentity.HashedProviderUserID).Updates(map[string]any{"provider_user_id": id.ProviderUserID, "hashed_provider_user_id": id.HashedProviderUserID}).Error; err != nil {
				return nil, false, false, err
			}

			// Now we should be able to load the identity.
			if err = tx.First(id).Error; err != nil {
				return nil, false, false, err
			}
		}
	} else if mode.enforced && id.UserID == 0 {
		return nil, false, false, scimUnprovisionedSignInError(0)
	}
	if err := c.decryptIdentity(ctx, id); err != nil {
		return nil, false, false, fmt.Errorf("failed to decrypt identity: %w", err)
	}

	// The identity's key as stored, for recording its first sign-in below.
	storedIdentityKey := []any{id.AuthProviderName, id.AuthProviderNamespace, id.HashedProviderUserID}

	var updateIdentity bool
	// This corrects the provider user ID and name to correct a bug introduced when re-encrypting all users and identities
	if id.Email != email || id.ProviderUserID != providerUserID || id.ProviderUsername != providerUsername {
		id.Email = email
		id.HashedEmail = hash.String(id.Email)
		id.ProviderUserID = providerUserID
		id.HashedProviderUserID = hash.String(id.ProviderUserID)
		id.ProviderUsername = providerUsername

		updateIdentity = true
	}

	// A user that sign-in creates has the email the identity provider asserts, so its explicit role applies. An
	// existing user replaces this one, and gets the explicit role of its own email below.
	newUserRole := role
	if r := c.HasExplicitRole(email); !newUserRole.HasRole(r) {
		newUserRole = newUserRole.SwitchBaseRole(r)
	}
	user := &types.User{
		ID:             id.UserID,
		Username:       id.ProviderUsername,
		HashedUsername: hash.String(id.ProviderUsername),
		Email:          id.Email,
		HashedEmail:    id.HashedEmail,
		VerifiedEmail:  &verified,
		Role:           newUserRole,
	}

	var created, roleRaised, checkForExistingUser bool
	// Whether SCIM provisioned the user is read with them, so that neither enforcement nor the protection of
	// SCIM-written profiles costs a query of its own.
	userQuery := tx.Model(new(types.User)).
		Select("users.*, "+
			"EXISTS (SELECT 1 FROM scim_user_bindings WHERE scim_user_bindings.user_id = users.id AND scim_user_bindings.retired_at IS NULL) AS scim_bound, "+
			"EXISTS (SELECT 1 FROM scim_user_bindings WHERE scim_user_bindings.user_id = users.id AND scim_user_bindings.retired_at IS NULL AND scim_user_bindings.connection_id = ?) AS scim_bound_to_connection",
			mode.connectionID).
		Where("users.deleted_at IS NULL")
	if user.ID != 0 {
		// Check for an existing user with this exact ID.
		userQuery = userQuery.Where("users.id = ?", user.ID)
		checkForExistingUser = true
	} else if verified {
		// Check for an existing user with this exact verified email address.
		// We check for both true and null values, because the email might have been verified before we started tracking verified emails.
		userQuery = userQuery.Where("users.hashed_email = ? and (users.verified_email = true or users.verified_email is null)", user.HashedEmail)
		checkForExistingUser = true
	}

	if checkForExistingUser {
		var rows []signInUserRow
		err := userQuery.Order("users.id").Limit(1).Scan(&rows).Error
		if err != nil {
			return nil, false, false, err
		}
		// Copy the user so that we don't have to decrypt unless the user already exists.
		u := *user
		if len(rows) == 0 {
			// Sign-in creates the user, so the mode must hold until the transaction ends.
			if err := mode.lock(tx); err != nil {
				return nil, false, false, err
			}
			if mode.enforced {
				return nil, false, false, scimUnprovisionedSignInError(user.ID)
			}

			// Clear user ID so that it can be auto-generated.
			u.ID = 0
			created = true
			if err = c.encryptUser(ctx, &u); err != nil {
				return nil, false, false, fmt.Errorf("failed to encrypt user: %w", err)
			}
			if err = c.createUser(tx, &u, userLimit); err != nil {
				return nil, false, false, err
			}

			// Copy the auto-generated values back to the user object.
			user.ID = u.ID
			user.CreatedAt = u.CreatedAt
			user.Role = u.Role
		} else {
			if mode.enforced && !rows[0].SCIMBoundToConnection {
				return nil, false, false, scimUnprovisionedSignInError(rows[0].ID)
			}

			u = rows[0].User
			if err := c.decryptUser(ctx, &u); err != nil {
				return nil, false, false, fmt.Errorf("failed to decrypt user: %w", err)
			}

			// Copy the decrypted existing user back.
			*user = u

			// We're using an existing user. See if there are any fields that need to be updated.
			var userChanged bool
			if !user.Role.HasRole(role) {
				user.Role = user.Role.SwitchBaseRole(role)
				userChanged = true
				roleRaised = true
			}

			// Explicit roles follow the user's email. SCIM writes the email of the users it has provisioned, and every
			// other check of explicit roles reads the stored email, so for those users it is the one that counts, not
			// the one the identity provider asserts at sign-in.
			explicitRoleEmail := email
			if rows[0].SCIMBound {
				explicitRoleEmail = user.Email
			}
			if r := c.HasExplicitRole(explicitRoleEmail); !user.Role.HasRole(r) {
				user.Role = user.Role.SwitchBaseRole(r)
				userChanged = true
				roleRaised = true
			}

			if user.Timezone == "" && timezone != "" {
				user.Timezone = timezone
				userChanged = true
			}

			if time.Since(user.LastActiveDay) > 24*time.Hour {
				user.LastActiveDay = time.Now().UTC().Truncate(24 * time.Hour)
				userChanged = true
			}

			// SCIM writes the profile of the users it has provisioned, whichever auth provider they sign in
			// through, so sign-in leaves it alone. When the user was read unprovisioned and the sign-in would change
			// the profile, the binding is read again with the user locked, so that SCIM cannot provision them
			// between the check and the write; SCIM locks the user too before it writes the profile.
			profileChanged := !rows[0].SCIMBound && (user.Username != id.ProviderUsername || user.Email != email)
			if profileChanged {
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", user.ID).Take(new(types.User)).Error; err != nil {
					return nil, false, false, fmt.Errorf("failed to lock user %d: %w", user.ID, err)
				}
				binding, err := activeSCIMUserBindingForUserTx(tx, user.ID, false)
				if err != nil {
					return nil, false, false, err
				}
				profileChanged = binding == nil
			}

			if profileChanged {
				user.Username = id.ProviderUsername
				user.HashedUsername = hash.String(user.Username)
				user.Email = email
				user.HashedEmail = hash.String(user.Email)
				userChanged = true
			}

			// Update the verified email status if needed.
			// This can happen in two cases:
			// 1. The user was created before we started tracking verified emails (user.VerifiedEmail is nil)
			// 2. The user was created before we started tracking verified emails, and associated with both a verified
			//    and unverified auth provider. They logged in with the unverified provider and we marked the email as unverified,
			//    but now they've logged in with the verified provider and we can mark the email as verified. (verified is true, but user.VerifiedEmail is false)
			if user.VerifiedEmail == nil || (verified && !*user.VerifiedEmail) {
				user.VerifiedEmail = &verified
				userChanged = true
			}

			if profileChanged {
				// Copy user so we don't have to decrypt
				u = *user
				if err := c.encryptUser(ctx, &u); err != nil {
					return nil, false, false, fmt.Errorf("failed to encrypt user: %w", err)
				}
				// The user was read without a lock, so its lifecycle state may be stale. Only lifecycle
				// operations write that state.
				if err = tx.Omit(types.UserLifecycleColumns...).Updates(u).Error; err != nil {
					return nil, false, false, err
				}
			} else if userChanged {
				// Only the columns that sign-in owns are written, so that a stale copy of the user, read without a
				// lock, cannot overwrite a profile that SCIM wrote in the meantime.
				if err = tx.Model(new(types.User)).Where("id = ?", user.ID).UpdateColumns(map[string]any{
					"role":            user.Role,
					"timezone":        user.Timezone,
					"last_active_day": user.LastActiveDay,
					"verified_email":  user.VerifiedEmail,
				}).Error; err != nil {
					return nil, false, false, err
				}
			}

			if roleRaised {
				// Role-dependent reconciliation must run, as it does when sign-in creates a user.
				if err := recordUserReconcileEvent(tx, user.ID, false); err != nil {
					return nil, false, false, err
				}
			}
		}
	} else {
		// Sign-in creates the user, so the mode must hold until the transaction ends.
		if err := mode.lock(tx); err != nil {
			return nil, false, false, err
		}
		if mode.enforced {
			return nil, false, false, scimUnprovisionedSignInError(0)
		}

		// Creating a new user
		created = true

		// Copy the user so we don't have to decrypt
		u := *user
		if err := c.encryptUser(ctx, &u); err != nil {
			return nil, false, false, fmt.Errorf("failed to encrypt user: %w", err)
		}
		if err := c.createUser(tx, &u, userLimit); err != nil {
			return nil, false, false, err
		}

		// Copy the values that were created instead of decrypting the whole object.
		user.ID = u.ID
		user.CreatedAt = u.CreatedAt
	}

	// Update the user ID saved on the identity if needed.
	if id.UserID != user.ID || updateIdentity {
		id.UserID = user.ID

		if err := c.encryptAndUpdateIdentity(ctx, tx, *id); err != nil {
			return nil, false, false, err
		}
	}

	// Record the identity's first sign-in once, without rewriting the rest of the identity. A disabled user is about
	// to be denied, which is not a sign-in: bootstrap relies on the marker to know that an Owner got in.
	if id.FirstSignInAt == nil && user.DisabledAt == nil {
		now := time.Now()
		if err := tx.Model(new(types.Identity)).
			Where("auth_provider_name = ? AND auth_provider_namespace = ? AND hashed_provider_user_id = ? AND first_sign_in_at IS NULL", storedIdentityKey...).
			UpdateColumn("first_sign_in_at", now).Error; err != nil {
			return nil, false, false, fmt.Errorf("failed to record the first sign-in of identity: %w", err)
		}
		id.FirstSignInAt = &now
	}

	return user, created, roleRaised, nil
}

func (c *Client) createUser(tx *gorm.DB, user *types.User, userLimit UserLimit) error {
	if userLimit.Unlimited {
		return tx.Create(user).Error
	}

	if err := lockUserCreation(tx); err != nil {
		return err
	}

	userCount, err := countUsersTowardLimit(tx)
	if err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}
	if userCount >= userLimit.Maximum {
		return newUserLimitError()
	}

	return tx.Create(user).Error
}

func countUsersTowardLimit(tx *gorm.DB) (int64, error) {
	var userCount int64
	err := tx.Model(new(types.User)).
		Where("deleted_at IS NULL").
		Where("hashed_username != ?", hash.String(system.BootstrapName)).
		Count(&userCount).Error
	return userCount, err
}

func lockUserCreation(tx *gorm.DB) error {
	// PostgreSQL installations may have multiple Obot replicas, so serialize
	// all operations that allocate user seats across them. SQLite operations
	// acquire its write lock before calling this helper.
	if tx.Name() == "postgres" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", userCreationAdvisoryLockID).Error; err != nil {
			return fmt.Errorf("failed to lock user creation: %w", err)
		}
	}
	return nil
}

func newUserLimitError() error {
	return types2.NewErrHTTP(
		http.StatusForbidden,
		"Unable to provision your account. Please contact your administrator.",
	)
}

// ensureIdentityProviderData fetches auth-provider data (the provider-native group lookup ID and
// the identity's group memberships) via HTTP and persists the results.
//
// It MUST be called outside of any open database transaction: the HTTP calls it makes to the auth
// provider would otherwise hold a pooled DB connection open across network round-trips. Each write
// below opens its own short transaction, so no transaction is ever held while an HTTP call is in
// flight. The identity + user rows are expected to already be committed by ensureIdentity.
func (c *Client) ensureIdentityProviderData(ctx context.Context, id *types.Identity) error {
	// Populate ProviderGroupLookupID from the auth provider's user info if not set.
	// This is the provider-native user ID used for group lookups, which may differ from the
	// OIDC sub claim stored in ProviderUserID (e.g. Entra returns an Azure AD GUID).
	if id.ProviderGroupLookupID == "" {
		// HTTP call, outside any transaction.
		if lookupID, err := c.fetchProviderGroupLookupID(ctx); err != nil {
			slog.Warn("failed to fetch provider group lookup ID", "error", err)
		} else if lookupID != "" {
			id.ProviderGroupLookupID = lookupID
			if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				return c.encryptAndUpdateIdentity(ctx, tx, *id)
			}); err != nil {
				return err
			}
		}
	}

	// Ensure groups and group memberships are up to date. ensureGroups makes its own HTTP call to
	// the auth provider (outside any transaction) and persists results in its own transaction.
	if err := c.ensureGroups(ctx, id); err != nil {
		return fmt.Errorf("failed to update groups for identity: %w", err)
	}

	return nil
}

// fetchProviderGroupLookupID calls the auth provider's /obot-get-user-info endpoint
// to get the provider-native user ID for group lookups.
func (c *Client) fetchProviderGroupLookupID(ctx context.Context) (string, error) {
	providerURL := auth.ProviderURLFromContext(ctx)
	accessToken := accesstoken.GetAccessToken(ctx)
	if providerURL == "" || accessToken == "" {
		return "", nil
	}

	profile, err := c.fetchUserProfile(ctx, providerURL, accessToken)
	if err != nil {
		return "", err
	}

	return extractProfileID(profile), nil
}

// extractProfileID extracts the "id" field from a provider's user info response.
// Handles both string and numeric ID types (GitHub returns int, others return string).
func extractProfileID(profile map[string]any) string {
	switch v := profile["id"].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%d", int64(v))
	default:
		return ""
	}
}

// encryptAndUpdateIdentity encrypts the identity and updates it in the database.
// It does not take a pointer so that the caller can use the identity object after the call without decrypting it.
func (c *Client) encryptAndUpdateIdentity(ctx context.Context, tx *gorm.DB, id types.Identity) error {
	if err := c.encryptIdentity(ctx, &id); err != nil {
		return fmt.Errorf("failed to encrypt identity: %w", err)
	}

	// Omit the group check column to prevent resetting the group refresh window
	if err := tx.Omit(groupsLastCheckedColumn).Updates(&id).Error; err != nil {
		return fmt.Errorf("failed to update identity: %w", err)
	}

	return nil
}

// RemoveIdentity deletes an identity from the database.
// The identity is deleted using UserID if set, otherwise ProviderUsername.
// The method is idempotent and ignores not-found errors, returning only unexpected errors.
//
// The identities of a user that SCIM has provisioned are never removed: the identity provider must
// deprovision the user, and deleting the user retires the binding first.
func (c *Client) RemoveIdentity(ctx context.Context, id *types.Identity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityQuery *gorm.DB

		// Build queries based on UserID or ProviderUsername
		if id.UserID != 0 {
			// Use UserID if set
			identityQuery = tx.Where("user_id = ?", id.UserID)
		} else {
			// Fall back to ProviderUsername
			identityQuery = tx.Where("hashed_provider_user_id = ?", id.HashedProviderUserID)
		}

		var userIDs []uint
		if err := identityQuery.Session(&gorm.Session{}).Model(new(types.Identity)).Distinct().Pluck("user_id", &userIDs).Error; err != nil {
			return err
		}
		for _, userID := range userIDs {
			if err := refuseSCIMProvisionedUserTx(tx, userID, "the identities of a user that SCIM has provisioned cannot be removed; remove the user's assignment in the identity provider, then delete the user"); err != nil {
				return err
			}
		}

		// Attempt to delete the identity
		if err := identityQuery.Delete(&types.Identity{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return nil
	})
}

// RemoveIdentityAndUser deletes an identity and the associated user from the database.
// The identity and user are deleted using UserID if set, otherwise ProviderUsername.
// The method is idempotent and ignores not-found errors, returning only unexpected errors.
func (c *Client) RemoveIdentityAndUser(ctx context.Context, id *types.Identity) (uint, error) {
	var user types.User
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identityQuery, userQuery *gorm.DB

		// Build queries based on UserID or ProviderUsername
		if id.UserID != 0 {
			// Use UserID if set
			identityQuery = tx.Where("user_id = ?", id.UserID)
			userQuery = tx.Where("id = ?", id.UserID)
		} else {
			// Fall back to ProviderUsername
			identityQuery = tx.Where("hashed_provider_user_id = ?", id.HashedProviderUserID)
			userQuery = tx.Where("hashed_username = ?", hash.String(id.ProviderUsername))
		}

		// Attempt to delete the identity
		if err := identityQuery.Delete(&types.Identity{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := userQuery.First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		if err := userQuery.Delete(&types.User{}).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		return nil
	})
	return user.ID, err
}

func (c *Client) createUserRoleChangeForNewUser(ctx context.Context, user *types.User) error {
	var defaultRole v1.UserDefaultRoleSetting
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &defaultRole); err != nil {
		return fmt.Errorf("failed to get default role setting: %w", err)
	}

	// Always create UserRoleChange for new users to trigger reconciliation
	// The handler will check effective role and create workspace if needed
	if err := c.storageClient.Create(ctx, &v1.UserRoleChange{
		GenerateName: system.UserRoleChangePrefix,
		Namespace:    system.DefaultNamespace,
		Spec: v1.UserRoleChangeSpec{
			UserID: user.ID,
		},
	}); err != nil {
		return fmt.Errorf("failed to create user role change event for new user %d: %w", user.ID, err)
	}

	return nil
}

func (c *Client) getDefaultRole(ctx context.Context) (types2.Role, error) {
	var defaultRole v1.UserDefaultRoleSetting
	if err := c.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &defaultRole); err != nil {
		return types2.RoleBasic, fmt.Errorf("failed to get default role setting: %w", err)
	}

	return defaultRole.Spec.Role, nil
}

func (c *Client) encryptIdentity(ctx context.Context, identity *types.Identity) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[identityGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = identityDataCtx(identity)
	)
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.ProviderUsername), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.ProviderUsername = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.Email), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.Email = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.ProviderUserID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.ProviderUserID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.ProviderGroupLookupID), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.ProviderGroupLookupID = base64.StdEncoding.EncodeToString(b)
	}
	if b, err = transformer.TransformToStorage(ctx, []byte(identity.IconURL), dataCtx); err != nil {
		errs = append(errs, err)
	} else {
		identity.IconURL = base64.StdEncoding.EncodeToString(b)
	}

	identity.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptIdentity(ctx context.Context, identity *types.Identity) error {
	if !identity.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[identityGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = identityDataCtx(identity)
	)

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.ProviderUsername)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.ProviderUsername))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.ProviderUsername = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.Email)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.Email))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.Email = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.ProviderUserID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.ProviderUserID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.ProviderUserID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.ProviderGroupLookupID)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.ProviderGroupLookupID))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.ProviderGroupLookupID = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	decoded = make([]byte, base64.StdEncoding.DecodedLen(len(identity.IconURL)))
	n, err = base64.StdEncoding.Decode(decoded, []byte(identity.IconURL))
	if err == nil {
		if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			identity.IconURL = string(out)
		}
	} else {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

func identityDataCtx(identity *types.Identity) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s/%s", identityGroupResource.String(), identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID))
}
