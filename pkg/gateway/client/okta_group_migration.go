package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"gorm.io/gorm"
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
// "okta/{name}" to "okta/{name}_{id}" format across all data stores.
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

	// Run the DB migration in a transaction.
	// We use INSERT ... ON CONFLICT DO NOTHING on the migrations row as a cross-replica lock:
	// only the replica whose insert succeeds (RowsAffected == 1) proceeds with the migration.
	// This is safe in multi-replica setups because the migrations row acts as an atomic claim.
	migrationRan := false
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Attempt to claim the migration. ON CONFLICT DO NOTHING means only one replica wins.
		result := tx.Exec("INSERT INTO migrations (name) VALUES (?) ON CONFLICT DO NOTHING", oktaGroupMigrationName)
		if result.Error != nil {
			return fmt.Errorf("okta migration error: failed to claim migration: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Another replica already completed or is completing the migration
			return nil
		}

		// We claimed the migration — proceed with the data migration.

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

			// Update group_memberships to point to the new group ID
			if err := tx.Model(&types.GroupMemberships{}).
				Where("group_id = ?", group.ID).
				Update("group_id", newID).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to update group_memberships for %s: %w", group.ID, err)
			}

			// Insert a new group row with the new ID, then delete the old one.
			// We insert-then-delete instead of update because the ID is part of the composite PK.
			newGroup := group
			newGroup.ID = newID
			if err := tx.Create(&newGroup).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to create new group %s: %w", newID, err)
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
		for oldID, newID := range idMap {
			if err := tx.Model(&types.GroupRoleAssignment{}).
				Where("group_name = ?", oldID).
				Update("group_name", newID).Error; err != nil {
				return fmt.Errorf("okta migration error: failed to update group_role_assignments for %s: %w", oldID, err)
			}
		}

		migrationRan = true
		return nil
	}); err != nil {
		return err
	}

	// Update CRDs (best-effort after DB migration succeeds)
	if migrationRan {
		c.migrateOktaGroupIDsInCRDs(ctx, idMap)
	}

	return nil
}

// migrateOktaGroupIDsInCRDs updates group subject IDs in AccessControlRule, SkillAccessRule, and ModelAccessPolicy CRDs.
func (c *Client) migrateOktaGroupIDsInCRDs(ctx context.Context, idMap map[string]string) {
	// Migrate AccessControlRules
	var acrList v1.AccessControlRuleList
	if err := c.storageClient.List(ctx, &acrList); err != nil {
		log.Warnf("Okta group migration: failed to list AccessControlRules: %v", err)
	} else {
		for i := range acrList.Items {
			if updateSubjects(acrList.Items[i].Spec.Manifest.Subjects, idMap) {
				if err := c.storageClient.Update(ctx, &acrList.Items[i]); err != nil {
					log.Warnf("Okta group migration: failed to update AccessControlRule %s: %v", acrList.Items[i].Name, err)
				}
			}
		}
	}

	// Migrate SkillAccessRules
	var sarList v1.SkillAccessRuleList
	if err := c.storageClient.List(ctx, &sarList); err != nil {
		log.Warnf("Okta group migration: failed to list SkillAccessRules: %v", err)
	} else {
		for i := range sarList.Items {
			if updateSubjects(sarList.Items[i].Spec.Manifest.Subjects, idMap) {
				if err := c.storageClient.Update(ctx, &sarList.Items[i]); err != nil {
					log.Warnf("Okta group migration: failed to update SkillAccessRule %s: %v", sarList.Items[i].Name, err)
				}
			}
		}
	}

	// Migrate ModelAccessPolicies
	var mapList v1.ModelAccessPolicyList
	if err := c.storageClient.List(ctx, &mapList); err != nil {
		log.Warnf("Okta group migration: failed to list ModelAccessPolicies: %v", err)
	} else {
		for i := range mapList.Items {
			if updateSubjects(mapList.Items[i].Spec.Manifest.Subjects, idMap) {
				if err := c.storageClient.Update(ctx, &mapList.Items[i]); err != nil {
					log.Warnf("Okta group migration: failed to update ModelAccessPolicy %s: %v", mapList.Items[i].Name, err)
				}
			}
		}
	}
}

// updateSubjects replaces old-format group IDs with new-format IDs in a subject list.
// Returns true if any subjects were modified.
func updateSubjects(subjects []types2.Subject, idMap map[string]string) bool {
	changed := false
	for i, s := range subjects {
		if s.Type != types2.SubjectTypeGroup {
			continue
		}
		if newID, ok := idMap[s.ID]; ok {
			subjects[i].ID = newID
			changed = true
		}
	}
	return changed
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
