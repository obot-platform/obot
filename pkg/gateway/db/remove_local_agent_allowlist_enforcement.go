package db

import (
	"fmt"

	"gorm.io/gorm"
)

// removeLocalAgentAllowlistEnforcement permanently deletes historical legacy
// enforcement decisions and the policy columns that backed allowlist
// evaluation. Every operation is guarded so upgrades are safe when a table or
// column is already absent.
func removeLocalAgentAllowlistEnforcement(tx *gorm.DB) error {
	migrator := tx.Migrator()
	if migrator.HasTable("enforcement_decision_logs") {
		if err := migrator.DropTable("enforcement_decision_logs"); err != nil {
			return fmt.Errorf("drop enforcement_decision_logs: %w", err)
		}
	}
	if migrator.HasTable("mdm_configurations") {
		for _, column := range []string{"enforcement_enabled", "enforcement_allowlist"} {
			if migrator.HasColumn("mdm_configurations", column) {
				// The names are fixed constants owned by this migration. Direct SQL
				// avoids GORM recreating a SQLite table from the current model, which
				// intentionally no longer describes these legacy columns.
				if err := tx.Exec("ALTER TABLE mdm_configurations DROP COLUMN " + column).Error; err != nil {
					return fmt.Errorf("drop mdm_configurations.%s: %w", column, err)
				}
			}
		}
	}
	return nil
}
