package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	gormDB      *gorm.DB
	sqlDB       *sql.DB
	autoMigrate bool
}

func New(dsn string, autoMigrate bool) (*DB, error) {
	var (
		gdb   gorm.Dialector
		conns = 1
	)
	switch {
	case strings.HasPrefix(dsn, "sqlite://"):
		gdb = sqlite.Open(strings.TrimPrefix(dsn, "sqlite://"))
	case strings.HasPrefix(dsn, "postgres://"):
		conns = 5
		gdb = postgres.Open(dsn)
	case strings.HasPrefix(dsn, "mysql://"):
		conns = 5
		gdb = mysql.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported driver: %s", dsn)
	}
	db, err := gorm.Open(gdb, &gorm.Config{
		SkipDefaultTransaction: true,
		Logger: logger.New(log.Default(), logger.Config{
			SlowThreshold: 200 * time.Millisecond,
			Colorful:      true,
			LogLevel:      logger.Silent,
		}),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetConnMaxLifetime(3 * time.Minute)
	sqlDB.SetMaxIdleConns(conns)
	sqlDB.SetMaxOpenConns(conns)

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
		types.Monitor{},
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
