package db

import (
	"fmt"

	"gorm.io/gorm"
)

func dropSessionsTable(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("sessions") {
		return nil
	}

	if err := tx.Migrator().DropTable("sessions"); err != nil {
		return fmt.Errorf("failed to drop sessions table: %w", err)
	}

	return nil
}
