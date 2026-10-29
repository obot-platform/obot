package client

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

const (
	// identityGroupLookupIDHashesMigrationName names the backfill of the hashed group lookup IDs of identities.
	identityGroupLookupIDHashesMigrationName = "identity_group_lookup_id_hashes"

	// identityGroupLookupIDBackfillBatchSize bounds the identities that each step of the backfill reads and writes.
	identityGroupLookupIDBackfillBatchSize = 500
)

// SCIMIdentitiesNotReadyError refuses to match SCIM users by group lookup ID before the backfill of their hashes has
// run, when SCIM could not find the identities that do not have one yet.
type SCIMIdentitiesNotReadyError struct{}

// SCIMUserWithoutGroupLookupID is a live user of an auth provider whose identity has no group lookup ID, so SCIM could
// not find them by it, and would provision them a duplicate user.
type SCIMUserWithoutGroupLookupID struct {
	UserID uint
	// Name is the user's email, or their username when they have none.
	Name     string
	Disabled bool
}

// SCIMGroupLookupIDsMissingError refuses to match SCIM users by group lookup ID while live users of the auth provider
// have none. Each must sign in (which causes the lookup ID to be stored) or be deleted.
type SCIMGroupLookupIDsMissingError struct {
	Users []SCIMUserWithoutGroupLookupID
}

func (e *SCIMIdentitiesNotReadyError) Error() string {
	return "Obot is still preparing identities for SCIM; try again shortly"
}

func (e *SCIMGroupLookupIDsMissingError) Error() string {
	return fmt.Sprintf("%d users must sign in before SCIM can find them by their group lookup IDs", len(e.Users))
}

// BackfillIdentityGroupLookupIDHashes stores the hash of the group lookup ID of every identity that was written
// without one, once. Identities written since the column was added already have it, because encryptIdentity sets it.
//
// The backfill reads the identities in batches, in primary key order, and holds no transaction between them. It is
// idempotent, so replicas starting together may both run it, and a run that fails is retried from the start.
func (c *Client) BackfillIdentityGroupLookupIDHashes(ctx context.Context) error {
	return c.MigrateKinmIfNotRun(ctx, identityGroupLookupIDHashesMigrationName, func() error {
		return c.backfillIdentityGroupLookupIDHashes(ctx, identityGroupLookupIDBackfillBatchSize, nil)
	})
}

// prepareIdentitiesForLookupMatching makes sure that every identity of the auth provider that has a group lookup ID
// can be found by its hash, before SCIM matches the provider's users by it. It refuses with
// *SCIMIdentitiesNotReadyError until the backfill has run once. It then backfills the provider's identities again,
// for those that replicas without the hash wrote since.
func (c *Client) prepareIdentitiesForLookupMatching(ctx context.Context, provider AuthProviderRef) error {
	if ready, err := c.IdentityGroupLookupIDHashesReady(ctx); err != nil {
		return err
	} else if !ready {
		return new(SCIMIdentitiesNotReadyError)
	}
	return c.backfillIdentityGroupLookupIDHashes(ctx, identityGroupLookupIDBackfillBatchSize, &provider)
}

// UsersWithoutGroupLookupID returns the live users of the auth provider whose identities have no group lookup ID, which
// must sign in, or be deleted, before SCIM can match the provider's users by it. It first prepares the provider's
// identities as prepareIdentitiesForLookupMatching does, and refuses with *SCIMIdentitiesNotReadyError until the
// backfill has run.
func (c *Client) UsersWithoutGroupLookupID(ctx context.Context, provider AuthProviderRef) ([]SCIMUserWithoutGroupLookupID, error) {
	if err := c.prepareIdentitiesForLookupMatching(ctx, provider); err != nil {
		return nil, err
	}
	return c.usersWithoutGroupLookupIDTx(ctx, c.db.WithContext(ctx), provider)
}

// usersWithoutGroupLookupIDTx returns the live users of the auth provider that have an identity of it without a group
// lookup ID, and none with one, oldest first. Every identity with a lookup ID has its hash once
// prepareIdentitiesForLookupMatching has run, so an identity without a hash has no lookup ID.
func (c *Client) usersWithoutGroupLookupIDTx(ctx context.Context, tx *gorm.DB, provider AuthProviderRef) ([]SCIMUserWithoutGroupLookupID, error) {
	identities := func(withLookupID bool) *gorm.DB {
		// NOT IN matches nothing when the subquery has a NULL, so NULLs are left out.
		query := tx.Model(new(types.Identity)).Select("user_id").
			Where("auth_provider_namespace = ? AND auth_provider_name = ? AND user_id IS NOT NULL", provider.Namespace, provider.Name)
		if withLookupID {
			return query.Where("hashed_provider_group_lookup_id IS NOT NULL AND hashed_provider_group_lookup_id != ''")
		}
		return query.Where("hashed_provider_group_lookup_id IS NULL OR hashed_provider_group_lookup_id = ''")
	}

	var users []types.User
	if err := tx.Where("deleted_at IS NULL").
		Where("id IN (?)", identities(false)).
		Where("id NOT IN (?)", identities(true)).
		Order("id").
		Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to list the users without a group lookup ID: %w", err)
	}

	missing := make([]SCIMUserWithoutGroupLookupID, 0, len(users))
	for i := range users {
		if err := c.decryptUser(ctx, &users[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt user: %w", err)
		}
		missing = append(missing, SCIMUserWithoutGroupLookupID{
			UserID:   users[i].ID,
			Name:     cmp.Or(users[i].Email, users[i].Username),
			Disabled: users[i].DisabledAt != nil,
		})
	}
	return missing, nil
}

// IdentityGroupLookupIDHashesReady reports whether the backfill of the hashed group lookup IDs of identities has run.
func (c *Client) IdentityGroupLookupIDHashesReady(ctx context.Context) (bool, error) {
	var migrations int64
	if err := c.db.WithContext(ctx).Model(new(types.Migration)).Where("name = ?", identityGroupLookupIDHashesMigrationName).Count(&migrations).Error; err != nil {
		return false, fmt.Errorf("failed to check the backfill of hashed identity group lookup IDs: %w", err)
	}
	return migrations > 0, nil
}

// backfillIdentityGroupLookupIDHashes stores the hashes of the identities without one, of provider, or of every auth
// provider when provider is nil.
func (c *Client) backfillIdentityGroupLookupIDHashes(ctx context.Context, batchSize int, provider *AuthProviderRef) error {
	var (
		db      = c.db.WithContext(ctx)
		after   *types.Identity
		scanned int
		updated int64
	)
	for {
		query := db.Where("hashed_provider_group_lookup_id IS NULL OR hashed_provider_group_lookup_id = ''").
			Order("auth_provider_namespace, auth_provider_name, hashed_provider_user_id").
			Limit(batchSize)
		if provider != nil {
			query = query.Where("auth_provider_namespace = ? AND auth_provider_name = ?", provider.Namespace, provider.Name)
		}
		if after != nil {
			query = query.Where("(auth_provider_namespace, auth_provider_name, hashed_provider_user_id) > (?, ?, ?)",
				after.AuthProviderNamespace, after.AuthProviderName, after.HashedProviderUserID)
		}

		var identities []types.Identity
		if err := query.Find(&identities).Error; err != nil {
			return fmt.Errorf("failed to list identities: %w", err)
		}
		if len(identities) == 0 {
			break
		}
		last := identities[len(identities)-1]
		after = &last

		if err := db.Transaction(func(tx *gorm.DB) error {
			for i := range identities {
				identity := &identities[i]
				// The lookup ID as stored, which may be encrypted, so that the write below applies only if no one has
				// changed it since it was read.
				stored := identity.ProviderGroupLookupID
				if err := c.decryptStoredIdentity(ctx, identity); err != nil {
					return fmt.Errorf("identity %s/%s/%s: %w", identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID, err)
				}
				if identity.ProviderGroupLookupID == "" {
					continue
				}

				result := tx.Model(new(types.Identity)).
					Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?",
						identity.AuthProviderNamespace, identity.AuthProviderName, identity.HashedProviderUserID).
					Where("provider_group_lookup_id = ?", stored).
					Where("hashed_provider_group_lookup_id IS NULL OR hashed_provider_group_lookup_id = ''").
					UpdateColumn("hashed_provider_group_lookup_id", hash.String(identity.ProviderGroupLookupID))
				if result.Error != nil {
					return fmt.Errorf("failed to store the hashed group lookup ID: %w", result.Error)
				}
				updated += result.RowsAffected
			}
			return nil
		}); err != nil {
			return err
		}

		scanned += len(identities)
		slog.Debug("Backfilling hashed identity group lookup IDs", "scanned", scanned, "updated", updated)
		if len(identities) < batchSize {
			break
		}
	}

	if scanned > 0 {
		slog.Info("Backfilled hashed identity group lookup IDs", "scanned", scanned, "updated", updated)
	}
	return nil
}
