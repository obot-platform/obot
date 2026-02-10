package db

import (
	"fmt"

	"gorm.io/gorm"
)

func dropObotMCPTokensTable(tx *gorm.DB) error {
	if !tx.Migrator().HasTable("obot_mcp_tokens") {
		return nil
	}

	if err := tx.Migrator().DropTable("obot_mcp_tokens"); err != nil {
		return fmt.Errorf("failed to drop obot_mcp_tokens table: %w", err)
	}

	return nil
}
