package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

// legacyVerifiedEmailUser is the users column of a previous release that the migration drops.
type legacyVerifiedEmailUser struct {
	VerifiedEmail *bool
}

func (legacyVerifiedEmailUser) TableName() string {
	return "users"
}

func TestAutoMigrateDropsUserVerifiedEmail(t *testing.T) {
	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = services.DB.SQLDB.Close() })

	database, err := New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway database: %v", err)
	}

	// Build the current schema, then add the column back, so that the database looks like one created by a previous
	// release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	if err := services.DB.DB.Migrator().AddColumn(&legacyVerifiedEmailUser{}, "VerifiedEmail"); err != nil {
		t.Fatalf("failed to add verified_email column: %v", err)
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	if services.DB.DB.Migrator().HasColumn(&types.User{}, "verified_email") {
		t.Fatal("users.verified_email still exists after migration")
	}
}
