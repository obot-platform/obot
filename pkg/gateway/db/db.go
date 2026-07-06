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

	if err = migrateRunTokenActivityInputOutput(tx); err != nil {
		return fmt.Errorf("failed to rename run_token_activities token columns: %w", err)
	}

	if err = dropRunTokenActivityPersonalToken(tx); err != nil {
		return fmt.Errorf("failed to drop run_token_activities personal_token column: %w", err)
	}

	if err = migrateUserDailyTokenLimits(tx); err != nil {
		return fmt.Errorf("failed to rename users daily token limit columns: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "drop_session_cookies", dropSessionCookiesTable); err != nil {
		return fmt.Errorf("failed to drop session_cookies table: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "remove_github_groups", removeGitHubGroups); err != nil {
		return fmt.Errorf("failed to remove GitHub groups: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "drop_obot_mcp_tokens", dropObotMCPTokensTable); err != nil {
		return fmt.Errorf("failed to drop obot_mcp_tokens table: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "drop_mcp_oauth_token_state_columns", dropMCPOAuthTokenStateColumns); err != nil {
		return fmt.Errorf("failed to drop state columns from mcp_oauth_tokens: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "drop_run_thread_run_states_tables", dropRunThreadAndRunStatesTables); err != nil {
		return fmt.Errorf("failed to drop run, thread, and run_states tables: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "apikey_skills_access_backfill", migrateAPIKeySkillsAccess); err != nil {
		return fmt.Errorf("failed to migrate API key skills access: %w", err)
	}

	if err := tx.AutoMigrate(
		types.AuthToken{},
		types.TokenRequest{},
		types.LLMAuditLog{},
		types.LLMProxyActivity{},
		types.User{},
		types.Identity{},
		types.Group{},
		types.GroupMemberships{},
		types.GroupRoleAssignment{},
		types.APIActivity{},
		types.Image{},
		types.RunTokenActivity{},
		types.MCPOAuthToken{},
		types.MCPOAuthPendingState{},
		types.MCPAuditLog{},
		types.TempSetupUser{},
		types.Property{},
		types.APIKey{},
		types.ServiceAccountAPIKey{},
		types.MessagePolicyViolation{},
		types.DeviceScan{},
		types.DeviceScanMCPServer{},
		types.DeviceScanSkill{},
		types.DeviceScanPlugin{},
		types.DeviceScanFile{},
		types.DeviceScanClient{},
		types.Credential{},
	); err != nil {
		return fmt.Errorf("failed to auto migrate gateway types: %w", err)
	}

	if err = migrateIfEntryNotFoundInMigrationsTable(tx, "mcp_audit_log_source_type_backfill", migrateMCPAuditLogSourceType); err != nil {
		return fmt.Errorf("failed to migrate mcp_audit_log source type: %w", err)
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
