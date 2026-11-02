package db

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"uuid"

	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

// newSQLiteTestDatabase opens a gateway database in an in-memory SQLite database.
func newSQLiteTestDatabase(t *testing.T) (*DB, *gorm.DB) {
	t.Helper()

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
	return database, services.DB.DB
}

// newPostgresTestDatabase opens a gateway database in a new schema of the PostgreSQL database that
// OBOT_TEST_POSTGRES_DSN names, and drops the schema when the test ends. The test is skipped when the variable is
// not set.
func newPostgresTestDatabase(t *testing.T, schemaPrefix string) (*DB, *gorm.DB) {
	t.Helper()

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
	schema := schemaPrefix + strings.ReplaceAll(uuid.New().String(), "-", "")
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
	return database, scoped.DB.DB
}
