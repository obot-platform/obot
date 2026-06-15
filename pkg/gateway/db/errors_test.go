package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

type duplicateKeyTestModel struct {
	ID    uint
	Value string `gorm:"uniqueIndex"`
}

func TestIsDuplicateKeyError(t *testing.T) {
	t.Run("gorm sentinel", func(t *testing.T) {
		if !IsDuplicateKeyError(fmt.Errorf("wrapped: %w", gorm.ErrDuplicatedKey)) {
			t.Fatal("expected wrapped gorm.ErrDuplicatedKey to be duplicate")
		}
	})

	t.Run("postgres unique violation", func(t *testing.T) {
		err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "23505"})
		if !IsDuplicateKeyError(err) {
			t.Fatal("expected PostgreSQL 23505 to be duplicate")
		}
	})

	t.Run("postgres other error", func(t *testing.T) {
		err := &pgconn.PgError{Code: "22001"}
		if IsDuplicateKeyError(err) {
			t.Fatal("expected PostgreSQL non-duplicate code to be ignored")
		}
	})

	t.Run("sqlite unique violation", func(t *testing.T) {
		services, err := sservices.New(sservices.Config{
			DSN: "sqlite://:memory:",
		})
		if err != nil {
			t.Fatalf("failed to create storage services: %v", err)
		}
		gormDB := services.DB.DB
		if err := gormDB.AutoMigrate(&duplicateKeyTestModel{}); err != nil {
			t.Fatalf("failed to migrate test table: %v", err)
		}
		if err := gormDB.Create(&duplicateKeyTestModel{Value: "same"}).Error; err != nil {
			t.Fatalf("failed to insert first row: %v", err)
		}
		err = gormDB.Create(&duplicateKeyTestModel{Value: "same"}).Error
		if err == nil {
			t.Fatal("expected duplicate insert to fail")
		}
		if !IsDuplicateKeyError(err) {
			t.Fatalf("expected SQLite unique violation to be duplicate: %v", err)
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		if IsDuplicateKeyError(errors.New("not a duplicate")) {
			t.Fatal("expected unrelated error to be ignored")
		}
	})
}
