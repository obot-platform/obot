package db

import (
	"context"
	"database/sql"

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

	// Only run PostgreSQL-specific migrations if using PostgreSQL
	if db.gormDB.Dialector.Name() == "postgres" {
		// Check if the identities table exists
		var exists bool
		if err := tx.Raw(`
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_name = 'identities'
			)
		`).Scan(&exists).Error; err != nil {
			return err
		}

		if exists {
			// Check if migration for Identity table is needed
			var needsIdentityMigration bool
			if err := tx.Raw(`
				SELECT COUNT(*) = 0 as needs_migration
				FROM information_schema.key_column_usage
				WHERE table_name = 'identities'
				AND constraint_name = 'identities_pkey'
				AND column_name = 'provider_user_id'
			`).Scan(&needsIdentityMigration).Error; err != nil {
				return err
			}

			if needsIdentityMigration {
				// Migration: Add provider_user_id to Identity table and update primary key
				if err := tx.Exec(`
				-- Drop existing primary key
				ALTER TABLE identities DROP CONSTRAINT identities_pkey;
				
				-- Add provider_user_id column
				ALTER TABLE identities ADD COLUMN provider_user_id text NOT NULL DEFAULT '';
				
				-- Set placeholder values for existing records
				UPDATE identities SET provider_user_id = 'OBOT_PLACEHOLDER_' || provider_username WHERE provider_user_id = '';
				
				-- Add new primary key
					ALTER TABLE identities ADD PRIMARY KEY (auth_provider_name, auth_provider_namespace, provider_user_id);
				`).Error; err != nil {
					return err
				}
			}
		}
	}

	return tx.AutoMigrate(
		types.AuthToken{},
		types.TokenRequest{},
		types.LLMProxyActivity{},
		types.LLMProvider{},
		types.Model{},
		types.OAuthTokenRequestChallenge{},
		types.OAuthTokenResponse{},
		types.User{},
		types.Identity{},
	)
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
