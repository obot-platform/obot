package db

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

// legacyVerifiedEmailUser is the users column of a previous release that the migration drops.
type legacyVerifiedEmailUser struct {
	VerifiedEmail *bool
}

func (legacyVerifiedEmailUser) TableName() string {
	return "users"
}

func TestUserVerifiedEmailMigrationOfAnExistingSQLiteDatabase(t *testing.T) {
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

	testUserVerifiedEmailMigration(t, database, services.DB.DB)
}

func TestUserVerifiedEmailMigrationOfAnExistingPostgresDatabase(t *testing.T) {
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
	schema := "obot_verified_email_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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

	testUserVerifiedEmailMigration(t, database, scoped.DB.DB)
}

// testUserVerifiedEmailMigration migrates a database whose users still have the verified_email column, and checks
// that the column is dropped and that the users are kept.
func testUserVerifiedEmailMigration(t *testing.T, database *DB, gormDB *gorm.DB) {
	t.Helper()

	// Build the current schema, then add the column back, so that the database looks like one created by a previous
	// release.
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to create the schema: %v", err)
	}
	if err := gormDB.Migrator().AddColumn(&legacyVerifiedEmailUser{}, "VerifiedEmail"); err != nil {
		t.Fatalf("failed to add verified_email column: %v", err)
	}
	if err := gormDB.Create(&types.User{
		Username:       "user",
		HashedUsername: "hashed-user",
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}
	if gormDB.Migrator().HasColumn(&types.User{}, "verified_email") {
		t.Fatal("users.verified_email still exists after migration")
	}
	var users int64
	if err := gormDB.Model(new(types.User)).Count(&users).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	if users != 1 {
		t.Fatalf("users after migration = %d, want 1", users)
	}
}
