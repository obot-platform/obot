package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// migrateAPIKeySkillsAccess adds the can_access_skills column to existing API keys
// and sets it to true for all existing rows. Prior to this change, all API keys
// implicitly had skills access, so existing keys are backfilled to preserve that behavior.
// New keys will default to false (set via the GORM tag on the struct field).
func migrateAPIKeySkillsAccess(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&types.APIKey{}) {
		return nil
	}

	// Add the column if it doesn't exist yet.
	if !tx.Migrator().HasColumn(&types.APIKey{}, "can_access_skills") {
		if err := tx.Migrator().AddColumn(&types.APIKey{}, "CanAccessSkills"); err != nil {
			return fmt.Errorf("failed to add can_access_skills column: %w", err)
		}
	}

	// Backfill: set all existing API keys to true since they previously had implicit access.
	// GORM will block an update with no where clause, so we use "1 = 1" to update all rows.
	if err := tx.Model(&types.APIKey{}).Where("1 = 1").Update("can_access_skills", true).Error; err != nil {
		return fmt.Errorf("failed to backfill can_access_skills: %w", err)
	}

	return nil
}
