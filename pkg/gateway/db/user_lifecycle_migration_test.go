package db

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

const (
	postgresTestDSNEnv = "OBOT_TEST_POSTGRES_DSN"
)

func TestUserLifecycleMigrationOfAnExistingSQLiteDatabase(t *testing.T) {
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

	testUserLifecycleMigration(t, database, services.DB.DB)
}

func TestUserLifecycleMigrationOfAnExistingPostgresDatabase(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresTestDSNEnv)
	}

	admin, err := sservices.New(sservices.Config{
		DSN: dsn,
	})
	if err != nil {
		t.Fatalf("failed to open PostgreSQL admin connection: %v", err)
	}
	schema := "obot_user_lifecycle_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = admin.DB.SQLDB.Close()
		t.Fatalf("failed to create schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("failed to drop schema: %v", err)
		}
		_ = admin.DB.SQLDB.Close()
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", postgresTestDSNEnv, err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()

	scoped, err := sservices.New(sservices.Config{
		DSN: u.String(),
	})
	if err != nil {
		t.Fatalf("failed to open scoped PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() { _ = scoped.DB.SQLDB.Close() })

	database, err := New(scoped.DB.DB, scoped.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway database: %v", err)
	}

	testUserLifecycleMigration(t, database, scoped.DB.DB)
}

// testUserLifecycleMigration migrates a database that holds users, identities, and memberships from before the
// lifecycle columns existed, and checks that existing users are enabled, existing identities are signed in, and
// nothing else changes.
func testUserLifecycleMigration(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	// Build the current schema, then remove what this change added, so that the database looks like one created
	// by the previous release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	migrator := gormDB.Migrator()
	for _, column := range []struct {
		model any
		name  string
	}{
		{
			model: &types.User{},
			name:  "disabled_at",
		},
		{
			model: &types.User{},
			name:  "disabled_reason",
		},
		{
			model: &types.Identity{},
			name:  "first_sign_in_at",
		},
	} {
		if err := migrator.DropColumn(column.model, column.name); err != nil {
			t.Fatalf("failed to drop column %s: %v", column.name, err)
		}
	}
	if err := migrator.DropTable(&types.UserLifecycleEvent{}); err != nil {
		t.Fatalf("failed to drop the lifecycle event table: %v", err)
	}
	if err := gormDB.Where("name = ?", "identity_first_sign_in_backfill").Delete(&types.Migration{}).Error; err != nil {
		t.Fatalf("failed to forget the backfill migration: %v", err)
	}

	deletedAt := time.Now().UTC().Truncate(time.Second)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  "INSERT INTO users (id, username, hashed_username, email, hashed_email, role) VALUES (?, ?, ?, ?, ?, ?)",
			args: []any{41, "00u-active", "active-hash", "active@example.com", "active-email-hash", apitypes.RoleOwner},
		},
		{
			sql:  "INSERT INTO users (id, username, hashed_username, email, hashed_email, role, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			args: []any{42, "00u-deleted_deleted_1", "deleted-hash", "deleted@example.com_deleted_1", "deleted-email-hash", apitypes.RoleBasic, deletedAt},
		},
		{
			sql:  "INSERT INTO identities (auth_provider_name, auth_provider_namespace, provider_user_id, hashed_provider_user_id, user_id) VALUES (?, ?, ?, ?, ?)",
			args: []any{"okta-auth-provider", "default", "00u-active", "hashed-00u-active", 41},
		},
		{
			sql:  "INSERT INTO identities (auth_provider_name, auth_provider_namespace, provider_user_id, hashed_provider_user_id, user_id) VALUES (?, ?, ?, ?, ?)",
			args: []any{"okta-auth-provider", "default", "00u-deleted", "hashed-00u-deleted", 42},
		},
		{
			sql:  "INSERT INTO groups (id, auth_provider_name, auth_provider_namespace, name) VALUES (?, ?, ?, ?)",
			args: []any{"okta/00g-existing", "okta-auth-provider", "default", "Existing Group"},
		},
		{
			sql:  "INSERT INTO group_memberships (user_id, group_id, created_at) VALUES (?, ?, ?)",
			args: []any{41, "okta/00g-existing", deletedAt},
		},
	} {
		if err := gormDB.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatalf("failed to insert existing data: %v", err)
		}
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	var users []types.User
	if err := gormDB.Order("id").Find(&users).Error; err != nil {
		t.Fatalf("failed to list users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("users after migration = %d, want 2", len(users))
	}
	if got := users[0]; got.ID != 41 || got.Username != "00u-active" || got.Role != apitypes.RoleOwner || got.DeletedAt != nil {
		t.Errorf("active user changed during migration: %+v", got)
	}
	if got := users[1]; got.ID != 42 || got.Username != "00u-deleted_deleted_1" || got.Role != apitypes.RoleBasic || got.DeletedAt == nil {
		t.Errorf("deleted user changed during migration: %+v", got)
	}
	for _, user := range users {
		if user.DisabledAt != nil || user.DisabledReason != "" {
			t.Errorf("user %d after migration has disabled at %v and reason %q, want enabled", user.ID, user.DisabledAt, user.DisabledReason)
		}
	}
	if got := users[0].Status(); got != apitypes.UserStatusActive {
		t.Errorf("active user status = %q, want %q", got, apitypes.UserStatusActive)
	}
	if got := users[1].Status(); got != apitypes.UserStatusDeleted {
		t.Errorf("deleted user status = %q, want %q", got, apitypes.UserStatusDeleted)
	}

	var identities []types.Identity
	if err := gormDB.Order("user_id").Find(&identities).Error; err != nil {
		t.Fatalf("failed to list identities: %v", err)
	}
	if len(identities) != 2 {
		t.Fatalf("identities after migration = %d, want 2", len(identities))
	}
	for _, identity := range identities {
		if identity.FirstSignInAt == nil {
			t.Errorf("identity of user %d after migration has no sign-in, want one", identity.UserID)
		}
	}

	var memberships []types.GroupMemberships
	if err := gormDB.Find(&memberships).Error; err != nil {
		t.Fatalf("failed to list memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].UserID != 41 || memberships[0].GroupID != "okta/00g-existing" {
		t.Errorf("memberships changed during migration: %+v", memberships)
	}

	if !migrator.HasTable(&types.UserLifecycleEvent{}) {
		t.Error("the lifecycle event table was not created")
	}
	if !migrator.HasIndex(&types.UserLifecycleEvent{}, "idx_user_lifecycle_events_pending") {
		t.Error("the index of pending lifecycle events was not created")
	}

	// The backfill runs once. An identity created later, before anyone signs in with it, keeps no sign-in across
	// restarts.
	if err := gormDB.Exec("INSERT INTO identities (auth_provider_name, auth_provider_namespace, provider_user_id, hashed_provider_user_id, user_id) VALUES (?, ?, ?, ?, ?)",
		"okta-auth-provider", "default", "00u-provisioned", "hashed-00u-provisioned", 41).Error; err != nil {
		t.Fatalf("failed to insert a new identity: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate a second time: %v", err)
	}
	var provisioned types.Identity
	if err := gormDB.Where("hashed_provider_user_id = ?", "hashed-00u-provisioned").Take(&provisioned).Error; err != nil {
		t.Fatalf("failed to read the new identity: %v", err)
	}
	if provisioned.FirstSignInAt != nil {
		t.Errorf("identity created after the migration has sign-in %v after a restart, want none", provisioned.FirstSignInAt)
	}
}
