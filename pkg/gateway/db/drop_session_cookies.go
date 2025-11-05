package db

import (
	"fmt"

	"gorm.io/gorm"
)

func dropSessionCookiesTable(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("session_cookies") {
		return nil
	}

	if err := tx.Migrator().DropTable("session_cookies"); err != nil {
		return fmt.Errorf("failed to drop session_cookies table: %w", err)
	}

	return nil
}
