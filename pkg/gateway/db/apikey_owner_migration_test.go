package db

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	apiKeyOwnerConstraint = "chk_api_keys_owner"
)

// apiKeyBeforeAgentOwners is the API key model of a release before agents could own API keys: every key had a
// user, and there was no agent_id column or owner constraint. It still has the hosted agent column that a later
// migration drops, which SQLite does by rebuilding the table after the owner constraint is added.
type apiKeyBeforeAgentOwners struct {
	types.APIKeyScopes

	ID                    uint   `gorm:"primaryKey;autoIncrement"`
	UserID                uint   `gorm:"index"`
	HostedAgentInstanceID string `gorm:"index"`
	ClaudeEnvironment     bool
	Name                  string
	Description           string
	HashedSecret          string
	CreatedAt             time.Time
	LastUsedAt            *time.Time
	ExpiresAt             *time.Time
	RevokedAt             *time.Time `gorm:"index"`
}

func (apiKeyBeforeAgentOwners) TableName() string {
	return "api_keys"
}

func TestAPIKeyOwnerConstraintOnSQLite(t *testing.T) {
	database, gormDB := newSQLiteTestDatabase(t)
	testAPIKeyOwnerConstraint(t, database, gormDB)
}

func TestAPIKeyOwnerConstraintOnPostgres(t *testing.T) {
	database, gormDB := newPostgresTestDatabase(t, "obot_api_key_owner_")
	testAPIKeyOwnerConstraint(t, database, gormDB)
}

func TestAPIKeyOwnerMigrationOfAnExistingSQLiteDatabase(t *testing.T) {
	database, gormDB := newSQLiteTestDatabase(t)
	testAPIKeyOwnerMigration(t, database, gormDB)
}

func TestAPIKeyOwnerMigrationOfAnExistingPostgresDatabase(t *testing.T) {
	database, gormDB := newPostgresTestDatabase(t, "obot_api_key_owner_migration_")
	testAPIKeyOwnerMigration(t, database, gormDB)
}

// testAPIKeyOwnerConstraint checks that a new database stores the keys of exactly one owner, and refuses a key with
// both owners or none, whether it is inserted that way or updated to be.
func testAPIKeyOwnerConstraint(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	assertAPIKeyOwnerSchema(t, gormDB)
	assertAPIKeyOwnerConstraintIsEnforced(t, gormDB)
}

// testAPIKeyOwnerMigration migrates an api_keys table of the release before agent owners, and checks that its keys
// are kept as the keys of their users, and that the owner constraint is added and enforced.
func testAPIKeyOwnerMigration(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	if err := gormDB.AutoMigrate(&apiKeyBeforeAgentOwners{}); err != nil {
		t.Fatalf("failed to create the previous api_keys table: %v", err)
	}
	revokedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	previous := []apiKeyBeforeAgentOwners{
		{
			UserID:       7,
			Name:         "cli",
			HashedSecret: "hash-1",
			CanAccessAPI: true,
			MCPServerIDs: []string{"*"},
		},
		{
			UserID:            8,
			Name:              "revoked",
			HashedSecret:      "hash-2",
			RevokedAt:         &revokedAt,
			CanAccessLLMProxy: true,
		},
	}
	if err := gormDB.Create(&previous).Error; err != nil {
		t.Fatalf("failed to create the previous keys: %v", err)
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	// Migrating again finds nothing to change.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate again: %v", err)
	}
	assertAPIKeyOwnerSchema(t, gormDB)
	if gormDB.Migrator().HasColumn(&types.APIKey{}, "hosted_agent_instance_id") {
		t.Error("api_keys still has the hosted agent column")
	}

	var keys []types.APIKey
	if err := gormDB.Order("id").Find(&keys).Error; err != nil {
		t.Fatalf("failed to read the migrated keys: %v", err)
	}
	if len(keys) != len(previous) {
		t.Fatalf("migrated keys = %d, want %d", len(keys), len(previous))
	}
	for i, key := range keys {
		want := previous[i]
		if key.ID != want.ID || key.UserID == nil || *key.UserID != want.UserID || key.AgentID != nil {
			t.Errorf("migrated key %d has user %v and agent %v, want user %d only", want.ID, key.UserID, key.AgentID, want.UserID)
		}
		if key.Name != want.Name || key.HashedSecret != want.HashedSecret || key.CanAccessAPI != want.CanAccessAPI ||
			key.CanAccessLLMProxy != want.CanAccessLLMProxy || len(key.MCPServerIDs) != len(want.MCPServerIDs) ||
			(key.RevokedAt == nil) != (want.RevokedAt == nil) {
			t.Errorf("migrated key %d = %+v, want %+v", want.ID, key, want)
		}
	}

	assertAPIKeyOwnerConstraintIsEnforced(t, gormDB)
}

// assertAPIKeyOwnerSchema checks that api_keys has a nullable user_id and agent_id, each indexed, and the owner
// constraint.
func assertAPIKeyOwnerSchema(t *testing.T, gormDB *gorm.DB) {
	t.Helper()

	migrator := gormDB.Migrator()
	if !migrator.HasConstraint(&types.APIKey{}, apiKeyOwnerConstraint) {
		t.Errorf("api_keys has no %s constraint", apiKeyOwnerConstraint)
	}
	for _, index := range []string{"idx_api_keys_user_id", "idx_api_keys_agent_id", "idx_api_keys_revoked_at"} {
		if !migrator.HasIndex(&types.APIKey{}, index) {
			t.Errorf("api_keys has no index %s", index)
		}
	}
	columns, err := migrator.ColumnTypes(&types.APIKey{})
	if err != nil {
		t.Fatalf("failed to read the api_keys columns: %v", err)
	}
	nullable := map[string]bool{}
	for _, column := range columns {
		if n, ok := column.Nullable(); ok {
			nullable[column.Name()] = n
		}
	}
	for _, column := range []string{"user_id", "agent_id"} {
		if n, ok := nullable[column]; !ok || !n {
			t.Errorf("api_keys.%s nullable = %v (known %v), want nullable", column, n, ok)
		}
	}
}

// assertAPIKeyOwnerConstraintIsEnforced checks that api_keys accepts a key of a user or of an agent, and refuses one
// with both owners or none.
func assertAPIKeyOwnerConstraintIsEnforced(t *testing.T, gormDB *gorm.DB) {
	t.Helper()

	for _, tt := range []struct {
		name    string
		userID  *uint
		agentID *uint
		wantErr bool
	}{
		{
			name:   "user key",
			userID: new(uint(100)),
		},
		{
			name:    "agent key",
			agentID: new(uint(100)),
		},
		{
			name:    "no owner",
			wantErr: true,
		},
		{
			name:    "both owners",
			userID:  new(uint(101)),
			agentID: new(uint(101)),
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := gormDB.Exec("INSERT INTO api_keys (user_id, agent_id, name, hashed_secret, created_at) VALUES (?, ?, ?, ?, ?)",
				tt.userID, tt.agentID, tt.name, "hash", time.Now()).Error
			if tt.wantErr && err == nil {
				t.Fatal("inserted a key that violates the owner constraint")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("failed to insert a key with one owner: %v", err)
			}
		})
	}

	// An existing key cannot gain a second owner or lose its only one.
	var userKeyID uint
	if err := gormDB.Raw("SELECT id FROM api_keys WHERE name = ?", "user key").Scan(&userKeyID).Error; err != nil || userKeyID == 0 {
		t.Fatalf("failed to find the user key: %d, %v", userKeyID, err)
	}
	for name, update := range map[string]map[string]any{
		"second owner": {"agent_id": 100},
		"no owner":     {"user_id": nil},
	} {
		if err := gormDB.Model(&types.APIKey{}).Where("id = ?", userKeyID).Updates(update).Error; err == nil {
			t.Errorf("updated a key to have %s", name)
		}
	}
}
