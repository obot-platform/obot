package client

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to auto-migrate: %v", err)
	}

	return &Client{
		db:                      db,
		llmAuditEntries:         make(chan llmAuditEntry, 6),
		llmAuditBatchSize:       3,
		auditLogCleanupInterval: 50 * time.Millisecond,
		auditLogDeleteBatchSize: 3,
	}
}

func insertAuditLog(t *testing.T, c *Client, createdAt time.Time) {
	t.Helper()
	entry := types.MCPAuditLog{CreatedAt: createdAt}
	if err := c.db.WithContext(context.Background()).Create(&entry).Error; err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}
}

func countAuditLogs(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(context.Background()).Model(&types.MCPAuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	return count
}

func insertLLMAuditLog(t *testing.T, c *Client, createdAt time.Time) {
	t.Helper()
	entry := types.LLMAuditLog{ID: uuid.NewString(), CreatedAt: createdAt}
	if err := c.db.WithContext(t.Context()).Create(&entry).Error; err != nil {
		t.Fatalf("failed to insert LLM audit log: %v", err)
	}
}

func countLLMAuditLogs(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.LLMAuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count LLM audit logs: %v", err)
	}
	return count
}

func TestDeleteOldAuditLogs(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -90)

	insertAuditLog(t, c, now.AddDate(0, 0, -100))  // old - should be deleted
	insertAuditLog(t, c, now.AddDate(0, 0, -91))   // old - should be deleted
	insertAuditLog(t, c, cutoff.Add(-time.Second)) // one second before cutoff - should be deleted
	insertAuditLog(t, c, cutoff)                   // exactly at cutoff boundary - should be kept (< not <=)
	insertAuditLog(t, c, now.AddDate(0, 0, -90))   // same day as cutoff but later in the day - should be kept
	insertAuditLog(t, c, now.AddDate(0, 0, -89))   // recent - should be kept
	insertAuditLog(t, c, now.AddDate(0, 0, -1))    // recent - should be kept
	insertAuditLog(t, c, now)                      // recent - should be kept

	if err := c.deleteOldMCPAuditLogs(ctx, now, 90); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countAuditLogs(t, c); got != 5 {
		t.Errorf("expected 5 audit logs after cleanup, got %d", got)
	}
}

func TestDeleteOldAuditLogsDisabled(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	now := time.Now().UTC()
	insertAuditLog(t, c, now.AddDate(0, 0, -200))
	insertAuditLog(t, c, now.AddDate(0, 0, -100))

	// retentionDays=0 should be a no-op
	if err := c.deleteOldMCPAuditLogs(ctx, now, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 audit logs (cleanup disabled), got %d", got)
	}
}

func TestDeleteOldAuditLogsBatching(t *testing.T) {
	c := newTestClient(t) // auditLogDeleteBatchSize = 3
	ctx := context.Background()

	now := time.Now().UTC()
	// Insert 7 old logs (requires 3 batches: 3+3+1) and 2 recent ones.
	for range 7 {
		insertAuditLog(t, c, now.AddDate(0, 0, -100))
	}
	insertAuditLog(t, c, now.AddDate(0, 0, -1))
	insertAuditLog(t, c, now)

	if err := c.deleteOldMCPAuditLogs(ctx, now, 90); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 audit logs after batched cleanup, got %d", got)
	}
}

func TestRunAuditLogCleanup(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertAuditLog(t, c, now.AddDate(0, 0, -100)) // old
	insertAuditLog(t, c, now.AddDate(0, 0, -1))   // recent

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.runMCPAuditLogCleanup(ctx, 90)

	// Wait until the cleanup has deleted old logs, or time out.
	deadline := time.Now().Add(2 * time.Second)
	var got int64
	for {
		got = countAuditLogs(t, c)
		if got == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for audit log cleanup, got %d logs", got)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	if got != 1 {
		t.Errorf("expected 1 audit log after cleanup loop, got %d", got)
	}
}

func TestRunAuditLogCleanupDisabled(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertAuditLog(t, c, now.AddDate(0, 0, -100))
	insertAuditLog(t, c, now.AddDate(0, 0, -1))

	// retentionDays=0 means the function returns immediately without cleanup.
	// Call synchronously — if it ever blocks, the test timeout will catch it.
	c.runMCPAuditLogCleanup(t.Context(), 0)

	if got := countAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 audit logs (cleanup disabled), got %d", got)
	}
}

func TestDeleteOldLLMAuditLogs(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -30)

	insertLLMAuditLog(t, c, now.AddDate(0, 0, -40))
	insertLLMAuditLog(t, c, cutoff.Add(-time.Second))
	insertLLMAuditLog(t, c, cutoff)
	insertLLMAuditLog(t, c, now.AddDate(0, 0, -1))

	if err := c.deleteOldLLMAuditLogs(ctx, now, 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countLLMAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 LLM audit logs after cleanup, got %d", got)
	}
}

func TestDeleteOldLLMAuditLogsDisabled(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	now := time.Now().UTC()
	insertLLMAuditLog(t, c, now.AddDate(0, 0, -40))
	insertLLMAuditLog(t, c, now)

	if err := c.deleteOldLLMAuditLogs(ctx, now, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countLLMAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 LLM audit logs (cleanup disabled), got %d", got)
	}
}

func TestDeleteOldLLMAuditLogsBatching(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	now := time.Now().UTC()
	for range 7 {
		insertLLMAuditLog(t, c, now.AddDate(0, 0, -40))
	}
	insertLLMAuditLog(t, c, now)

	if err := c.deleteOldLLMAuditLogs(ctx, now, 30); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countLLMAuditLogs(t, c); got != 1 {
		t.Errorf("expected 1 LLM audit log after batched cleanup, got %d", got)
	}
}
