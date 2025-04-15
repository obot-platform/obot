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

	// Only run PostgreSQL-specific migrations if using PostgreSQL
	if db.gormDB.Name() == "postgres" {
		if err = addAuthProviderNameAndNamespace(tx); err != nil {
			return fmt.Errorf("failed to add auth provider name and namespace: %w", err)
		}
	}

	if err = addIdentityAndUserHashedFields(tx); err != nil {
		return fmt.Errorf("failed to add identity and user hashed fields: %w", err)
	}

	if err := tx.AutoMigrate(&GptscriptCredential{}); err != nil {
		return fmt.Errorf("failed to auto migrate GptscriptCredential: %w", err)
	}

	return tx.AutoMigrate(
		types.AuthToken{},
		types.TokenRequest{},
		types.LLMProxyActivity{},
		types.OAuthTokenRequestChallenge{},
		types.OAuthTokenResponse{},
		types.User{},
		types.Identity{},
		types.APIActivity{},
		types.Image{},
		types.RunState{},
		types.FileScannerConfig{},
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
