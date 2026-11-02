package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

func TestInitialOwnerGrantBackfillOfAnExistingSQLiteDatabase(t *testing.T) {
	database, gormDB := newSQLiteTestDatabase(t)
	testInitialOwnerGrantBackfill(t, database, gormDB)
}

func TestInitialOwnerGrantBackfillOfAnExistingPostgresDatabase(t *testing.T) {
	database, gormDB := newPostgresTestDatabase(t, "obot_initial_owner_grant_")
	testInitialOwnerGrantBackfill(t, database, gormDB)
}

// testInitialOwnerGrantBackfill migrates a database from before Owner grants were recorded, and checks that the grant
// is recorded for a provisioned owner who signed in and still has a setup token, and for no other account.
func testInitialOwnerGrantBackfill(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	// Build the current schema, then forget the backfill, so that the database looks like one created by a previous
	// release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	if err := gormDB.Where("name = ?", "initial_owner_grant_backfill").Delete(new(types.Migration)).Error; err != nil {
		t.Fatalf("failed to forget the backfill: %v", err)
	}

	localAccount := func(email, setupTokenHash string) {
		t.Helper()
		if err := gormDB.Create(&types.LocalAuthUser{
			Email:          email,
			HashedEmail:    hash.String(email),
			SetupTokenHash: setupTokenHash,
		}).Error; err != nil {
			t.Fatalf("failed to create local account %s: %v", email, err)
		}
	}
	identity := func(provider, providerUserID string) {
		t.Helper()
		if err := gormDB.Create(&types.Identity{
			AuthProviderName:      provider,
			AuthProviderNamespace: system.DefaultNamespace,
			ProviderUsername:      providerUserID,
			ProviderUserID:        providerUserID,
			HashedProviderUserID:  hash.String(providerUserID),
		}).Error; err != nil {
			t.Fatalf("failed to create %s identity %s: %v", provider, providerUserID, err)
		}
	}

	// A provisioned owner who signed in, and was made an Owner then, before finishing setup.
	localAccount("signed-in@example.com", "signed-in-token-hash")
	identity(system.LocalAuthProvider, "signed-in@example.com")
	// A provisioned owner who has not signed in. Another provider's identity with the same email does not count.
	localAccount("pending@example.com", "pending-token-hash")
	identity("google-auth-provider", "pending@example.com")
	// An account that signed in and has no setup token.
	localAccount("member@example.com", "")
	identity(system.LocalAuthProvider, "member@example.com")

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	for email, wantGranted := range map[string]bool{
		"signed-in@example.com": true,
		"pending@example.com":   false,
		"member@example.com":    false,
	} {
		var account types.LocalAuthUser
		if err := gormDB.Where("hashed_email = ?", hash.String(email)).First(&account).Error; err != nil {
			t.Fatalf("failed to read local account %s: %v", email, err)
		}
		if granted := account.OwnerGrantedAt != nil; granted != wantGranted {
			t.Errorf("grant of %s recorded = %t, want %t", email, granted, wantGranted)
		}
	}
}
