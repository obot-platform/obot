package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func removeGitHubGroups(tx *gorm.DB) error {
	// Check if tables exist
	if !tx.Migrator().HasTable(&types.GroupMemberships{}) || !tx.Migrator().HasTable(&types.Group{}) {
		return nil
	}

	// Delete from group_memberships first (foreign key constraint order)
	if err := tx.Where("group_id LIKE ?", "github/%").Delete(&types.GroupMemberships{}).Error; err != nil {
		return fmt.Errorf("failed to delete GitHub group memberships: %w", err)
	}

	// Delete from groups
	if err := tx.Where("id LIKE ?", "github/%").Delete(&types.Group{}).Error; err != nil {
		return fmt.Errorf("failed to delete GitHub groups: %w", err)
	}

	return nil
}
