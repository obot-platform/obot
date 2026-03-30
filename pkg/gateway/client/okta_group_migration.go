package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const oktaGroupMigrationName = "okta_group_id_migration"

type oktaGroupMigrationMapping struct {
	OldID string `json:"oldID"`
	NewID string `json:"newID"`
}

// runOktaGroupIDMigrationOnce runs the Okta group ID migration at most once.
// It is safe for concurrent callers; subsequent calls are no-ops after success.
// On failure, the migration will be retried on the next authentication request.
func (c *Client) runOktaGroupIDMigrationOnce(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string) error {
	c.oktaGroupMigrationMu.Lock()
	defer c.oktaGroupMigrationMu.Unlock()

	if c.oktaGroupMigrationDone {
		return nil
	}

	if err := c.migrateOktaGroupIDs(ctx, authProviderURL, authProviderNamespace, authProviderName); err != nil {
		return err
	}

	c.oktaGroupMigrationDone = true
	return nil
}

// migrateOktaGroupIDs performs the one-time migration of Okta group IDs from
// "okta/{name}" format to "okta/{group ID}" format across all data stores.
func (c *Client) migrateOktaGroupIDs(ctx context.Context, authProviderURL, authProviderNamespace, authProviderName string) error {
	// Fast path: check if migration has already been completed (avoids fetching the mapping)
	var migration types.Migration
	err := c.db.WithContext(ctx).Where("name = ?", oktaGroupMigrationName).First(&migration).Error
	if err == nil {
		// Migration already done
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check migration status: %w", err)
	}

	// Fetch the old→new ID mapping from the auth provider
	mappings, err := fetchGroupMigrationMapping(ctx, authProviderURL)
	if err != nil {
		return fmt.Errorf("failed to fetch group migration mapping: %w", err)
	}
	if mappings == nil {
		// Auth provider returned 404 (older version without migration support) — skip
		return nil
	}

	idMap := make(map[string]string, len(mappings))
	for _, m := range mappings {
		idMap[m.OldID] = m.NewID
	}

	log.Infof("Running Okta group ID migration")

	// Claim and run the data migration in a single transaction.
	// With PostgreSQL READ COMMITTED, another replica trying to INSERT the same claim row
	// will block until this transaction commits or rolls back:
	//   - On commit: the other replica's INSERT gets DO NOTHING and it skips the migration.
	//   - On rollback (crash/error): the claim disappears and the other replica retries.
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&types.Migration{Name: oktaGroupMigrationName})
		if result.Error != nil {
			return fmt.Errorf("okta migration error: failed to claim migration: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Another replica already completed the migration
			return nil
		}

		// Load existing okta groups for this auth provider
		var groups []types.Group
		if err := tx.Where("auth_provider_namespace = ? AND auth_provider_name = ?",
			authProviderNamespace, authProviderName).
			Find(&groups).Error; err != nil {
			return fmt.Errorf("okta migration error: failed to load groups: %w", err)
		}

		for _, group := range groups {
			newID, ok := idMap[group.ID]
			if !ok {
				continue
			}

			// Update group_memberships to point to the new group ID.
			// If a user already has a new-format membership (e.g. they logged in after the
			// auth provider was updated), delete the old-format row instead of updating
			// to avoid (user_id, group_id) primary key conflicts.
			if err := tx.Where("group_id = ? AND user_id IN (?)",
				group.ID,
				tx.Table("group_memberships").Select("user_id").Where("group_id = ?", newID),
			).Delete(&types.GroupMemberships{}).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to delete duplicate group_memberships for %s: %w", group.ID, err)
			}
			if err := tx.Model(&types.GroupMemberships{}).
				Where("group_id = ?", group.ID).
				Update("group_id", newID).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to update group_memberships for %s: %w", group.ID, err)
			}

			// Upsert the new group row, then delete the old one.
			// We upsert instead of plain create to handle cases where a new-format group
			// was already created (e.g. a user logged in after the auth provider was updated
			// but before the migration ran).
			newGroup := group
			newGroup.ID = newID
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}, {Name: "auth_provider_name"}, {Name: "auth_provider_namespace"}},
				DoUpdates: clause.AssignmentColumns([]string{"name", "icon_url"}),
			}).Create(&newGroup).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to upsert new group %s: %w", newID, err)
			}
			if err := tx.Where("id = ? AND auth_provider_namespace = ? AND auth_provider_name = ?",
				group.ID, group.AuthProviderNamespace, group.AuthProviderName).
				Delete(&types.Group{}).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to delete old group %s: %w", group.ID, err)
			}
		}

		// Update group_role_assignments directly from the mapping.
		// This catches assignments for groups that don't exist in the groups table
		// (e.g. when an admin assigned a role to a group but no user from that group has logged in).
		// If a new-format assignment already exists, delete the old one instead of updating
		// to avoid primary key conflicts.
		for oldID, newID := range idMap {
			var existingNew types.GroupRoleAssignment
			err := tx.Where("group_name = ?", newID).First(&existingNew).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("okta migration error: failed to check existing group_role_assignment for %s: %w", newID, err)
			}
			if err == nil {
				// New-format assignment already exists — just delete the old one
				if delErr := tx.Where("group_name = ?", oldID).Delete(&types.GroupRoleAssignment{}).Error; delErr != nil {
					return fmt.Errorf("okta migration error: failed to delete old group_role_assignment for %s: %w", oldID, delErr)
				}
				continue
			}
			if err := tx.Model(&types.GroupRoleAssignment{}).
				Where("group_name = ?", oldID).
				Update("group_name", newID).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to update group_role_assignments for %s: %w", oldID, err)
			}
		}

		// Create an OktaGroupMigration task object so the controller handler
		// migrates the CRDs with automatic retries on failure.
		// This is inside the transaction so that only the replica holding the lock
		// creates the object, and if creation fails the claim row is rolled back,
		// allowing a retry on the next authentication request.
		if err := c.storageClient.Create(ctx, &v1.OktaGroupMigration{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.OktaGroupMigrationPrefix,
				Namespace:    system.DefaultNamespace,
			},
			Spec: v1.OktaGroupMigrationSpec{IDMapping: idMap},
		}); err != nil {
			return fmt.Errorf("failed to create OktaGroupMigration task: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	return nil
}

// fetchGroupMigrationMapping fetches the old→new group ID mapping from the auth provider.
// Returns nil, nil if the auth provider does not support this endpoint (404).
func fetchGroupMigrationMapping(ctx context.Context, authProviderURL string) ([]oktaGroupMigrationMapping, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authProviderURL+"/obot-get-group-migration-mapping", nil)
	if err != nil {
		return nil, fmt.Errorf("okta migration error: failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("okta migration error: failed to fetch migration mapping: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Old auth provider version without migration support
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("okta migration error: migration mapping endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var mappings []oktaGroupMigrationMapping
	if err := json.NewDecoder(resp.Body).Decode(&mappings); err != nil {
		return nil, fmt.Errorf("okta migration error: failed to decode migration mapping: %w", err)
	}

	return mappings, nil
}
