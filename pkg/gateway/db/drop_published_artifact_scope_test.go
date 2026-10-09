package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

func TestDropPublishedArtifactScopeColumns(t *testing.T) {
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
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}

	tx := services.DB.DB
	for _, table := range []string{"api_keys", "token_requests"} {
		if err := tx.Exec("ALTER TABLE " + table + " ADD COLUMN can_access_published_artifacts boolean NOT NULL DEFAULT false").Error; err != nil {
			t.Fatalf("failed to add the column to %s: %v", table, err)
		}
	}

	// Running twice shows the migration is a no-op once the columns are gone.
	for range 2 {
		if err := dropPublishedArtifactScopeColumns(tx); err != nil {
			t.Fatalf("dropPublishedArtifactScopeColumns() error = %v", err)
		}
	}

	for _, model := range []any{&types.APIKey{}, &types.TokenRequest{}} {
		if tx.Migrator().HasColumn(model, "can_access_published_artifacts") {
			t.Errorf("%T still has can_access_published_artifacts", model)
		}
	}
}
