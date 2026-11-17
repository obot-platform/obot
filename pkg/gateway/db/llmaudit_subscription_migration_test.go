package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLLMAuditSubscriptionMigrationSQLite(t *testing.T) {
	database, sql := newSQLiteTestDatabase(t)
	testLLMAuditSubscriptionMigration(t, database, sql)
}

func TestLLMAuditSubscriptionMigrationPostgres(t *testing.T) {
	database, sql := newPostgresTestDatabase(t, "obot_audit_subscription_")
	testLLMAuditSubscriptionMigration(t, database, sql)
}

func testLLMAuditSubscriptionMigration(t *testing.T, database *DB, sql *gorm.DB) {
	t.Helper()
	require.NoError(t, sql.Exec("CREATE TABLE llm_audit_logs (id TEXT PRIMARY KEY, claude_subscription BOOLEAN NOT NULL DEFAULT FALSE)").Error)
	require.NoError(t, sql.Exec("INSERT INTO llm_audit_logs (id, claude_subscription) VALUES ('disabled', FALSE), ('enabled', TRUE)").Error)
	require.NoError(t, database.AutoMigrate())
	require.True(t, sql.Migrator().HasColumn(&types.LLMAuditLog{}, "claude_subscription"))

	require.NoError(t, sql.Create(&types.LLMAuditLog{ID: "new", Subscription: true}).Error)
	// Simulate an old replica inserting after the new replica has migrated the schema.
	require.NoError(t, sql.Exec("INSERT INTO llm_audit_logs (id, claude_subscription) VALUES ('old', TRUE)").Error)
	require.NoError(t, database.AutoMigrate(), "a second startup must preserve the migrated values")
	var logs []types.LLMAuditLog
	require.NoError(t, sql.Order("id").Find(&logs).Error)
	require.Len(t, logs, 4)
	require.Equal(t, "disabled", logs[0].ID)
	require.False(t, logs[0].Subscription)
	require.Equal(t, "enabled", logs[1].ID)
	require.True(t, logs[1].Subscription)
	require.Equal(t, "new", logs[2].ID)
	require.True(t, logs[2].Subscription)
	require.Equal(t, "old", logs[3].ID)
	require.True(t, logs[3].Subscription)
}
