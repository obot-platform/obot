package client

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestMigrateIfNotRunRollsBackMarkerOnFailureAndRunsOnce(t *testing.T) {
	c := newTestClient(t)
	runs := 0
	migration := func(tx *gorm.DB) error {
		runs++
		var markerCount int64
		if err := tx.Model(&gatewaytypes.Migration{}).Where("name = ?", "test_atomic_marker").Count(&markerCount).Error; err != nil {
			return err
		}
		if markerCount != 1 {
			return errors.New("migration marker was not claimed before the migration body")
		}
		if runs == 1 {
			return errors.New("injected migration failure")
		}
		return nil
	}

	if err := c.migrateIfNotRun(t.Context(), "test_atomic_marker", migration); err == nil {
		t.Fatal("migration unexpectedly succeeded")
	}
	if err := c.migrateIfNotRun(t.Context(), "test_atomic_marker", migration); err != nil {
		t.Fatalf("migration retry failed: %v", err)
	}
	if err := c.migrateIfNotRun(t.Context(), "test_atomic_marker", migration); err != nil {
		t.Fatalf("completed migration was not idempotent: %v", err)
	}
	if runs != 2 {
		t.Fatalf("migration body ran %d times, want 2", runs)
	}
}

func TestMigrateUnencryptedCredentialsPreservesAndEncryptsLegacyRows(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			credentialGroupResource: staticOAuthTestTransformer{},
		},
	}
	legacy := gatewaytypes.Credential{
		Context: "mcp-oauth-entry-1",
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "legacy-client", "CLIENT_SECRET": "legacy-secret"},
	}
	if err := c.db.WithContext(t.Context()).Create(&legacy).Error; err != nil {
		t.Fatalf("seed plaintext credential: %v", err)
	}

	if err := c.MigrateUnencryptedCredentials(t.Context()); err != nil {
		t.Fatalf("migrate plaintext credentials: %v", err)
	}
	var stored gatewaytypes.Credential
	if err := c.db.WithContext(t.Context()).First(&stored, legacy.ID).Error; err != nil {
		t.Fatalf("read migrated credential: %v", err)
	}
	encoded, err := json.Marshal(stored.Secrets)
	if err != nil {
		t.Fatalf("marshal stored secrets: %v", err)
	}
	if !stored.Encrypted || strings.Contains(string(encoded), "legacy-client") || strings.Contains(string(encoded), "legacy-secret") {
		t.Fatalf("legacy credential remained plaintext: encrypted=%v secrets=%s", stored.Encrypted, encoded)
	}
	revealed, err := c.RevealCredential(t.Context(), []string{legacy.Context}, legacy.Name)
	if err != nil {
		t.Fatalf("reveal migrated credential: %v", err)
	}
	if revealed.Secrets["CLIENT_ID"] != "legacy-client" || revealed.Secrets["CLIENT_SECRET"] != "legacy-secret" {
		t.Fatalf("migrated credential changed: %#v", revealed.Secrets)
	}
	if err := c.MigrateUnencryptedCredentials(t.Context()); err != nil {
		t.Fatalf("migration was not idempotent: %v", err)
	}
}

func TestMigrateToolReferenceCredentialContexts(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
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
