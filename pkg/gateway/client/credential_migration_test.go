package client

import (
	"context"
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestMigrateToolReferenceCredentialContexts(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()
	db := c.db.WithContext(ctx)

	if err := db.Exec("CREATE TABLE toolreference (uid text)").Error; err != nil {
		t.Fatalf("failed to create toolreference table: %v", err)
	}
	if err := db.Exec("INSERT INTO toolreference (uid) VALUES (?), (?), (?)", "tool-uid", "tool-uid", "missing-uid").Error; err != nil {
		t.Fatalf("failed to insert toolreference rows: %v", err)
	}

	migrated := gatewaytypes.Credential{
		Context: "tool-uid",
		Name:    "credential-name",
		Secrets: map[string]string{"API_KEY": "secret"},
	}
	unchanged := gatewaytypes.Credential{
		Context: "other-context",
		Name:    "other-name",
		Secrets: map[string]string{"TOKEN": "other"},
	}
	if err := db.Create(&migrated).Error; err != nil {
		t.Fatalf("failed to create credential to migrate: %v", err)
	}
	if err := db.Create(&unchanged).Error; err != nil {
		t.Fatalf("failed to create credential to leave unchanged: %v", err)
	}

	if err := c.MigrateToolReferenceCredentialContexts(ctx); err != nil {
		t.Fatalf("failed to migrate toolreference credential contexts: %v", err)
	}

	var got gatewaytypes.Credential
	if err := db.Where("name = ?", "credential-name").First(&got).Error; err != nil {
		t.Fatalf("failed to get migrated credential: %v", err)
	}
	if got.Context != got.Name {
		t.Fatalf("expected migrated credential context %q to match name %q", got.Context, got.Name)
	}
	if got.Secrets["API_KEY"] != "secret" {
		t.Fatalf("expected migrated credential secrets to be preserved, got %#v", got.Secrets)
	}

	var gotUnchanged gatewaytypes.Credential
	if err := db.Where("name = ?", "other-name").First(&gotUnchanged).Error; err != nil {
		t.Fatalf("failed to get unchanged credential: %v", err)
	}
	if gotUnchanged.Context != "other-context" {
		t.Fatalf("expected unrelated credential context to remain unchanged, got %q", gotUnchanged.Context)
	}

	if db.Migrator().HasTable("toolreference") {
		t.Fatal("expected toolreference table to be dropped")
	}

	var migration gatewaytypes.Migration
	if err := db.Where("name = ?", toolReferenceCredentialContextMigrationName).First(&migration).Error; err != nil {
		t.Fatalf("expected migration record to be created: %v", err)
	}

	if err := c.MigrateToolReferenceCredentialContexts(ctx); err != nil {
		t.Fatalf("expected migration to be idempotent: %v", err)
	}
}
