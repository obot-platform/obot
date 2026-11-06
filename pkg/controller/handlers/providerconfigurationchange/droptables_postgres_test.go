package providerconfigurationchange

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestDropAuthProviderTables checks that deconfiguring an auth provider drops its session tables and the tables its
// manifest declares, and nothing else. It needs OBOT_TEST_POSTGRES_DSN.
func TestDropAuthProviderTables(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set OBOT_TEST_POSTGRES_DSN to run the PostgreSQL tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	prefix := fmt.Sprintf("droptest%d_", time.Now().UnixNano())
	owned := []string{prefix + "sessions", prefix + "session_locks", prefix + "user_groups", prefix + "seen_groups"}
	// An undeclared table that shares the prefix, and another provider's sessions, must survive.
	kept := []string{prefix + "undeclared", "other" + prefix + "sessions"}
	for _, table := range append(append([]string{}, owned...), kept...) {
		require.NoError(t, db.Exec("CREATE TABLE "+table+" (id integer)").Error)
		t.Cleanup(func() { _ = db.Exec("DROP TABLE IF EXISTS " + table).Error })
	}

	authProvider := v1.AuthProvider{
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				PostgresTablePrefix: prefix,
				PostgresTables:      []string{"user_groups", "seen_groups"},
			},
		},
	}
	require.NoError(t, dropAuthProviderTables(authProvider, dsn))

	for _, table := range owned {
		assert.False(t, db.Migrator().HasTable(table), "table %s should have been dropped", table)
	}
	for _, table := range kept {
		assert.True(t, db.Migrator().HasTable(table), "table %s should have been kept", table)
	}

	// Dropping again, with the tables already gone, succeeds.
	require.NoError(t, dropAuthProviderTables(authProvider, dsn))
}

// TestDropAuthProviderTablesRejectsInvalidNames checks that a declared table outside the provider's namespace is
// refused before anything is dropped.
func TestDropAuthProviderTablesRejectsInvalidNames(t *testing.T) {
	authProvider := v1.AuthProvider{
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				PostgresTablePrefix: "ping_",
				PostgresTables:      []string{"x; DROP TABLE users"},
			},
		},
	}
	// The DSN is never dialed: the names are checked first.
	assert.Error(t, dropAuthProviderTables(authProvider, "postgres://unused.invalid/obot"))

	// Without a database, there is nothing to drop.
	assert.NoError(t, dropAuthProviderTables(authProvider, ""))
}
