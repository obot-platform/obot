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

func TestSCIMStorageOfAnExistingSQLiteDatabase(t *testing.T) {
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

	testSCIMStorage(t, database, services.DB.DB)
}

func TestSCIMStorageOfAnExistingPostgresDatabase(t *testing.T) {
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
	schema := "obot_scim_storage_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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

	testSCIMStorage(t, database, scoped.DB.DB)
}

// testSCIMStorage migrates a database from before the SCIM tables existed, checks that the existing data is
// unchanged, and then checks the constraints of the new tables.
func testSCIMStorage(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	// Build the current schema, then remove the SCIM tables, so that the database looks like one created by the
	// previous release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	migrator := gormDB.Migrator()
	scimTables := []any{
		&types.SCIMConnection{},
		&types.SCIMUserBinding{},
		&types.SCIMGroupBinding{},
		&types.SCIMPendingGroupDeletion{},
		&types.SCIMGroupSubjectCleanup{},
		&types.SCIMRequestFailure{},
	}
	for _, table := range scimTables {
		if err := migrator.DropTable(table); err != nil {
			t.Fatalf("failed to drop the table of %T: %v", table, err)
		}
	}
	if err := migrator.DropIndex(&types.GroupMemberships{}, "GroupID"); err != nil {
		t.Fatalf("failed to drop the group ID index of group memberships: %v", err)
	}

	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{
			sql:  "INSERT INTO users (id, username, hashed_username, email, hashed_email, role) VALUES (?, ?, ?, ?, ?, ?)",
			args: []any{41, "00u-owner", "owner-hash", "owner@example.com", "owner-email-hash", apitypes.RoleOwner},
		},
		{
			sql:  "INSERT INTO users (id, username, hashed_username, email, hashed_email, role) VALUES (?, ?, ?, ?, ?, ?)",
			args: []any{42, "00u-basic", "basic-hash", "basic@example.com", "basic-email-hash", apitypes.RoleBasic},
		},
		{
			sql:  "INSERT INTO groups (id, auth_provider_name, auth_provider_namespace, name) VALUES (?, ?, ?, ?)",
			args: []any{"okta/00g-existing", "okta-auth-provider", "default", "Existing Group"},
		},
		{
			sql:  "INSERT INTO groups (id, auth_provider_name, auth_provider_namespace, name) VALUES (?, ?, ?, ?)",
			args: []any{"okta/00g-other", "okta-auth-provider", "default", "Other Group"},
		},
	} {
		if err := gormDB.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatalf("failed to insert existing data: %v", err)
		}
	}

	// Migrating twice is the same as migrating once.
	for range 2 {
		if err := database.AutoMigrate(); err != nil {
			t.Fatalf("failed to migrate: %v", err)
		}
	}
	for _, table := range scimTables {
		if !migrator.HasTable(table) {
			t.Fatalf("the table of %T was not created", table)
		}
	}
	if !migrator.HasIndex(&types.GroupMemberships{}, "GroupID") {
		t.Fatal("the group ID index of group memberships was not created")
	}
	if !migrator.HasIndex(&types.SCIMUserBinding{}, "idx_scim_user_bindings_list") {
		t.Fatal("the index of the pages of SCIM users was not created")
	}

	var users, groups int64
	if err := gormDB.Model(new(types.User)).Count(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Model(new(types.Group)).Count(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if users != 2 || groups != 2 {
		t.Fatalf("got %d users and %d groups after migration, want 2 and 2", users, groups)
	}

	insert := func(value any) error {
		t.Helper()
		// Each insert runs in its own transaction, so that a refused one leaves PostgreSQL usable.
		return gormDB.Transaction(func(tx *gorm.DB) error {
			return tx.Create(value).Error
		})
	}
	mustInsert := func(value any) {
		t.Helper()
		if err := insert(value); err != nil {
			t.Fatalf("failed to insert %+v: %v", value, err)
		}
	}
	mustRefuse := func(what string, value any) {
		t.Helper()
		if err := insert(value); err == nil {
			t.Fatalf("%s was stored", what)
		}
	}

	now := time.Now()
	mustInsert(&types.SCIMConnection{
		ID:                    "conn-1",
		AdapterType:           "okta",
		Origin:                types.SCIMConnectionOriginMigrated,
		AuthProviderNamespace: "default",
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		State:                 types.SCIMConnectionStateConnected,
		EnabledAt:             now,
	})
	mustRefuse("a second connection for the same auth provider", &types.SCIMConnection{
		ID:                    "conn-2",
		AdapterType:           "okta",
		Origin:                types.SCIMConnectionOriginMigrated,
		AuthProviderNamespace: "default",
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		State:                 types.SCIMConnectionStateConnected,
		EnabledAt:             now,
	})

	userBinding := func(id string, userID uint, nativeID, userName string, retiredAt *time.Time) *types.SCIMUserBinding {
		return &types.SCIMUserBinding{
			ID:                 id,
			ConnectionID:       "conn-1",
			UserID:             userID,
			HashedNativeUserID: nativeID,
			HashedUserName:     userName,
			Active:             true,
			RetiredAt:          retiredAt,
		}
	}
	mustInsert(userBinding("user-1", 41, "native-1", "name-1", nil))
	mustRefuse("a second binding of the same native user ID", userBinding("user-2", 42, "native-1", "name-2", nil))
	mustRefuse("a second binding of the same user", userBinding("user-3", 41, "native-3", "name-3", nil))
	mustRefuse("a second binding of the same userName", userBinding("user-4", 42, "native-4", "name-1", nil))
	mustRefuse("a reused SCIM user ID", userBinding("user-1", 42, "native-5", "name-5", &now))

	// Retired bindings are tombstones that constrain nothing but their own ID.
	mustInsert(userBinding("user-6", 42, "native-6", "name-6", &now))
	mustInsert(userBinding("user-7", 42, "native-6", "name-6", nil))

	groupBinding := func(id, groupID, name string, retiredAt *time.Time) *types.SCIMGroupBinding {
		return &types.SCIMGroupBinding{
			ID:                    id,
			ConnectionID:          "conn-1",
			GroupID:               groupID,
			NormalizedDisplayName: name,
			Origin:                types.SCIMGroupBindingOriginExisting,
			RetiredAt:             retiredAt,
		}
	}
	mustInsert(groupBinding("group-1", "okta/00g-existing", "existing group", nil))
	mustRefuse("a second binding of the same group", groupBinding("group-2", "okta/00g-existing", "renamed", nil))
	mustRefuse("a second bound group with the same name", groupBinding("group-3", "okta/00g-other", "existing group", nil))
	mustInsert(groupBinding("group-4", "okta/00g-other", "other group", &now))
	mustInsert(groupBinding("group-5", "okta/00g-other", "other group", nil))

	mustInsert(&types.SCIMPendingGroupDeletion{
		GroupID:      "okta/00g-unreferenced",
		ConnectionID: "conn-1",
	})
	mustRefuse("a second pending deletion of the same group", &types.SCIMPendingGroupDeletion{
		GroupID:      "okta/00g-unreferenced",
		ConnectionID: "conn-1",
	})
}
