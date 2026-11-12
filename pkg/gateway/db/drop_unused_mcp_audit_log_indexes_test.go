package db

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func TestDropUnusedMCPAuditLogIndexes(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		database, tx := newSQLiteTestDatabase(t)
		testDropUnusedMCPAuditLogIndexes(t, database, tx)
	})
	t.Run("postgres", func(t *testing.T) {
		database, tx := newPostgresTestDatabase(t, "obot_drop_audit_idx_")
		testDropUnusedMCPAuditLogIndexes(t, database, tx)
	})
}

func testDropUnusedMCPAuditLogIndexes(t *testing.T, database *DB, tx *gorm.DB) {
	t.Helper()

	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}
	migrator := tx.Migrator()

	// A new database must not get the indexes at all.
	for _, index := range unusedMCPAuditLogIndexes {
		if migrator.HasIndex(&types.MCPAuditLog{}, index) {
			t.Errorf("AutoMigrate created %s on a new database", index)
		}
	}

	// Recreate them as an earlier version would have left them. This is also what happens when a
	// replica on an earlier version restarts after they were dropped.
	for _, index := range unusedMCPAuditLogIndexes {
		column := strings.TrimPrefix(index, "idx_mcp_audit_logs_")
		if err := tx.Exec("CREATE INDEX " + index + " ON mcp_audit_logs (" + column + ")").Error; err != nil {
			t.Fatalf("failed to create %s: %v", index, err)
		}
	}

	// The next startup drops them. Running twice shows it is a no-op once the indexes are gone.
	for range 2 {
		if err := database.AutoMigrate(); err != nil {
			t.Fatalf("failed to migrate gateway database: %v", err)
		}
	}

	for _, index := range unusedMCPAuditLogIndexes {
		if migrator.HasIndex(&types.MCPAuditLog{}, index) {
			t.Errorf("%s still exists", index)
		}
	}

	// Only one coming back, in any later startup, is dropped as well.
	if err := tx.Exec("CREATE INDEX idx_mcp_audit_logs_turn_id ON mcp_audit_logs (turn_id)").Error; err != nil {
		t.Fatalf("failed to create idx_mcp_audit_logs_turn_id: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}
	if migrator.HasIndex(&types.MCPAuditLog{}, "idx_mcp_audit_logs_turn_id") {
		t.Error("idx_mcp_audit_logs_turn_id still exists after it was created again")
	}
	// The indexes that queries rely on must survive.
	for _, index := range []string{
		"idx_mcp_audit_logs_created_at",
		"idx_mcp_audit_logs_occurred_at",
		"idx_mcp_audit_logs_idempotency_key",
		"idx_mcp_audit_logs_proxy_exchange_id",
		"idx_mcp_audit_logs_request_id",
		"idx_mcp_audit_logs_target_parent_name",
		"idx_mcp_audit_api_key_created",
		"idx_mcp_audit_tool_calls_daily",
	} {
		if !migrator.HasIndex(&types.MCPAuditLog{}, index) {
			t.Errorf("%s was dropped", index)
		}
	}
}
