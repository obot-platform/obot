package db

import (
	"context"
	"database/sql"
	"net/http"

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

	return tx.AutoMigrate(
		types.AuthToken{},
		types.TokenRequest{},
		types.LLMProxyActivity{},
		types.AuthProvider{},
		types.LLMProvider{},
		types.Model{},
		types.OAuthTokenRequestChallenge{},
		types.OAuthTokenResponse{},
		types.User{},
		types.Identity{},
	)
}

func (db *DB) Check(w http.ResponseWriter, _ *http.Request) {
	if err := db.sqlDB.Ping(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	_, _ = w.Write([]byte(`{"status": "ok"}`))
}

func (db *DB) Close() error {
	return db.sqlDB.Close()
}

func (db *DB) WithContext(ctx context.Context) *gorm.DB {
	return db.gormDB.WithContext(ctx)
}
