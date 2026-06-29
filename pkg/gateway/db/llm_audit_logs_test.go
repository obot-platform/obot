package db

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

func TestLLMAuditLogPartitionStartFromName(t *testing.T) {
	from, ok := llmAuditLogPartitionStartFromName("llm_audit_logs_2026_06")
	if !ok {
		t.Fatal("expected partition name to parse")
	}

	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !from.Equal(want) {
		t.Fatalf("expected %s, got %s", want, from)
	}
}

func TestCreateLLMAuditLogPartitionSQL(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	want := `CREATE TABLE IF NOT EXISTS "llm_audit_logs_2026_06" PARTITION OF "llm_audit_logs" FOR VALUES FROM ($1) TO ($2)`
	if got := createLLMAuditLogPartitionSQL(from); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestLLMAuditLogPartitionSettings(t *testing.T) {
	if llmAuditLogFuturePartitions != 2 {
		t.Fatalf("expected 2 future partitions, got %d", llmAuditLogFuturePartitions)
	}
}

func TestLLMAuditLogPartitionStart(t *testing.T) {
	now := time.Date(2026, 6, 29, 23, 0, 0, 0, time.UTC)
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if got := llmAuditLogPartitionStart(now); !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestAutoMigrateCreatesLLMAuditLogsForSQLite(t *testing.T) {
	db := newSQLiteGatewayDB(t)

	if !db.WithContext(t.Context()).Migrator().HasTable(&types.LLMAuditLog{}) {
		t.Fatal("expected llm_audit_logs table")
	}
}

func TestMaintainLLMAuditLogsDeletesOldRowsForSQLite(t *testing.T) {
	db := newSQLiteGatewayDB(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	logs := []types.LLMAuditLog{
		{ID: "old", CreatedAt: now.AddDate(0, 0, -31)},
		{ID: "recent", CreatedAt: now.AddDate(0, 0, -1)},
	}
	if err := db.WithContext(ctx).Create(&logs).Error; err != nil {
		t.Fatalf("failed to insert LLM audit logs: %v", err)
	}

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return MaintainLLMAuditLogPartitions(ctx, tx, 30, now)
	}); err != nil {
		t.Fatalf("failed to maintain LLM audit logs: %v", err)
	}

	var count int64
	if err := db.WithContext(ctx).Model(&types.LLMAuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count LLM audit logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 LLM audit log, got %d", count)
	}
}

func newSQLiteGatewayDB(t *testing.T) *DB {
	t.Helper()

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to auto-migrate: %v", err)
	}
	return db
}
