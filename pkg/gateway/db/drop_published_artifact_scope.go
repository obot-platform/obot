package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// dropPublishedArtifactScopeColumns drops the scope of the removed published artifacts feature from API keys and
// the token requests that create them.
func dropPublishedArtifactScopeColumns(tx *gorm.DB) error {
	migrator := tx.Migrator()
	for _, model := range []any{&types.APIKey{}, &types.TokenRequest{}} {
		if !migrator.HasColumn(model, "can_access_published_artifacts") {
			continue
		}
		stmt := &gorm.Statement{DB: tx}
		if err := stmt.Parse(model); err != nil {
			return fmt.Errorf("failed to parse %T: %w", model, err)
		}
		// The column is no longer part of the model, which gorm's SQLite migrator needs to drop it.
		if err := tx.Exec("ALTER TABLE ? DROP COLUMN can_access_published_artifacts", gorm.Expr(tx.Statement.Quote(stmt.Schema.Table))).Error; err != nil {
			return fmt.Errorf("failed to drop can_access_published_artifacts from %s: %w", stmt.Schema.Table, err)
		}
	}
	return nil
}
