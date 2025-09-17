package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type DB struct {
	gormDB      *gorm.DB
	sqlDB       *sql.DB
	autoMigrate bool
}

func New(db *gorm.DB, sqlDB *sql.DB, autoMigrate bool) (*DB, error) {
	return &DB{
		gormDB:      db,
		sqlDB:       sqlDB,
		autoMigrate: autoMigrate,
	}, nil
}

func (db *DB) AutoMigrate() (err error) {
	if !db.autoMigrate {
		return nil
	}

	tx := db.gormDB.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	if err = tx.AutoMigrate(&types.Migration{}); err != nil {
		return fmt.Errorf("failed to migrate migration table: %w", err)
	}

	// Only run PostgreSQL-specific migrations if using PostgreSQL
	if db.gormDB.Name() == "postgres" {
		if err = addAuthProviderNameAndNamespace(tx); err != nil {
			return fmt.Errorf("failed to add auth provider name and namespace: %w", err)
		}
	}

	if err = addIdentityAndUserHashedFields(tx); err != nil {
		return fmt.Errorf("failed to add identity and user hashed fields: %w", err)
	}

	if err = dropMCPOAuthTokensTableForUserIDPrimaryKey(tx); err != nil {
		return fmt.Errorf("failed to drop mcp_server_instance table: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "auditor_user_role", migrateUserRoles); err != nil {
		return fmt.Errorf("failed to migrate user roles: %w", err)
	}

	if err = migrateMCPAuditLogClientInfo(tx); err != nil {
		return fmt.Errorf("failed to migrate mcp_audit_log client info: %w", err)
	}

	if err := tx.AutoMigrate(&GptscriptCredential{}); err != nil {
		return fmt.Errorf("failed to auto migrate GptscriptCredential: %w", err)
	}

	if err := tx.AutoMigrate(
		types.AuthToken{},
		types.TokenRequest{},
		types.LLMProxyActivity{},
		types.OAuthTokenRequestChallenge{},
		types.OAuthTokenResponse{},
		types.User{},
		types.Identity{},
		types.Group{},
		types.GroupMemberships{},
		types.APIActivity{},
		types.Image{},
		types.RunState{},
		types.FileScannerConfig{},
		types.RunTokenActivity{},
		types.MCPOAuthToken{},
		types.MCPAuditLog{},
		types.SessionCookie{},
	); err != nil {
		return fmt.Errorf("failed to auto migrate gateway types: %w", err)
	}

	// MIGRATION: replace mcp_server_instance with mcp_id as the new primary key.
	// First, check to se if the mcp_server_instance column still exists.
	if exists := tx.Migrator().HasColumn(&types.MCPOAuthToken{}, "mcp_server_instance"); exists {
		// If the column exists, we need to drop this table and recreate it.
		// It will delete all entries in the process, which is what we want.
		if err := tx.Migrator().DropTable(&types.MCPOAuthToken{}); err != nil {
			return fmt.Errorf("failed to drop mcp_server_instance table: %w", err)
		}
		if err := tx.AutoMigrate(&types.MCPOAuthToken{}); err != nil {
			return fmt.Errorf("failed to auto migrate mcp_server_instance table: %w", err)
		}
	}

	return nil
}

func (db *DB) Check(ctx context.Context) error {
	return db.sqlDB.PingContext(ctx)
}

func (db *DB) Close() error {
	return db.sqlDB.Close()
}

func (db *DB) WithContext(ctx context.Context) *gorm.DB {
	return db.gormDB.WithContext(ctx)
}
