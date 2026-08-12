package client

import (
	"context"
	"errors"
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
		db:                        db,
		llmAuditEntries:           make(chan llmAuditEntry, 6),
		llmAuditBatchSize:         3,
		llmAuditEnabled:           true,
		auditLogCleanupInterval:   50 * time.Millisecond,
		auditLogDeleteBatchSize:   3,
		deviceScanCleanupInterval: 50 * time.Millisecond,
		deviceScanDeleteBatchSize: 3,
	}
}

func insertAuditLog(t *testing.T, c *Client, createdAt time.Time) {
	t.Helper()
	entry := types.MCPAuditLog{CreatedAt: createdAt}
	if err := c.db.WithContext(t.Context()).Create(&entry).Error; err != nil {
		t.Fatalf("failed to insert audit log: %v", err)
	}
}

func countAuditLogs(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.MCPAuditLog{}).Count(&count).Error; err != nil {
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

func insertAPIKey(t *testing.T, c *Client, revokedAt *time.Time) uint {
	t.Helper()
	entry := types.APIKey{
		UserID:    1,
		Name:      "retention-test",
		CreatedAt: time.Now().UTC().AddDate(0, 0, -200),
		RevokedAt: revokedAt,
	}
	if err := c.db.WithContext(t.Context()).Create(&entry).Error; err != nil {
		t.Fatalf("failed to insert API key: %v", err)
	}
	return entry.ID
}

func countAPIKeys(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.APIKey{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count API keys: %v", err)
	}
	return count
}

func TestAPIKeyRetentionDays(t *testing.T) {
	for _, tt := range []struct {
		name          string
		mcpDays       int
		llmDays       int
		wantRetention int
	}{
		{name: "MCP is longer", mcpDays: 90, llmDays: 30, wantRetention: 90},
		{name: "LLM is longer", mcpDays: 30, llmDays: 90, wantRetention: 90},
		{name: "equal", mcpDays: 90, llmDays: 90, wantRetention: 90},
		{name: "MCP cleanup disabled", mcpDays: 0, llmDays: 90, wantRetention: 0},
		{name: "LLM cleanup disabled", mcpDays: 90, llmDays: 0, wantRetention: 0},
		{name: "negative disables cleanup", mcpDays: -1, llmDays: 90, wantRetention: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiKeyRetentionDays(tt.mcpDays, tt.llmDays); got != tt.wantRetention {
				t.Fatalf("apiKeyRetentionDays(%d, %d) = %d, want %d", tt.mcpDays, tt.llmDays, got, tt.wantRetention)
			}
		})
	}
}

func TestDeleteOldRevokedAPIKeys(t *testing.T) {
	c := newTestClient(t)
	now := time.Now().UTC()
	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -90)
	old := cutoff.Add(-time.Second)
	atCutoff := cutoff
	recent := cutoff.Add(time.Second)

	insertAPIKey(t, c, nil)
	insertAPIKey(t, c, &old)
	insertAPIKey(t, c, &atCutoff)
	insertAPIKey(t, c, &recent)

	if err := c.deleteOldRevokedAPIKeys(t.Context(), now, 90); err != nil {
		t.Fatalf("delete old revoked API keys: %v", err)
	}
	if got := countAPIKeys(t, c); got != 3 {
		t.Fatalf("API keys after cleanup = %d, want 3", got)
	}
}

func TestDeleteOldRevokedAPIKeysBatching(t *testing.T) {
	c := newTestClient(t)
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -100)
	for range 7 {
		insertAPIKey(t, c, &old)
	}
	recent := now.AddDate(0, 0, -1)
	insertAPIKey(t, c, &recent)

	if err := c.deleteOldRevokedAPIKeys(t.Context(), now, 90); err != nil {
		t.Fatalf("delete old revoked API keys: %v", err)
	}
	if got := countAPIKeys(t, c); got != 1 {
		t.Fatalf("API keys after batched cleanup = %d, want 1", got)
	}
}

func TestDeleteOldRevokedAPIKeysDisabled(t *testing.T) {
	c := newTestClient(t)
	old := time.Now().UTC().AddDate(0, 0, -200)
	insertAPIKey(t, c, &old)

	if err := c.deleteOldRevokedAPIKeys(t.Context(), time.Now().UTC(), 0); err != nil {
		t.Fatalf("disabled cleanup: %v", err)
	}
	if got := countAPIKeys(t, c); got != 1 {
		t.Fatalf("API keys after disabled cleanup = %d, want 1", got)
	}
}

func TestRetentionDeletesPropagateCanceledContext(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	now := time.Now().UTC()

	for _, tt := range []struct {
		name   string
		delete func() error
	}{
		{name: "MCP audit logs", delete: func() error { return c.deleteOldMCPAuditLogs(ctx, now, 90) }},
		{name: "LLM audit logs", delete: func() error { return c.deleteOldLLMAuditLogs(ctx, now, 90) }},
		{name: "revoked API keys", delete: func() error { return c.deleteOldRevokedAPIKeys(ctx, now, 90) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.delete(); !errors.Is(err, context.Canceled) {
				t.Fatalf("cleanup error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestDeleteOldAuditLogs(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

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
	ctx := t.Context()

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
	ctx := t.Context()

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

func TestRunRetentionCleanup(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertAuditLog(t, c, now.AddDate(0, 0, -100)) // old
	insertAuditLog(t, c, now.AddDate(0, 0, -1))   // recent
	insertLLMAuditLog(t, c, now.AddDate(0, 0, -100))
	insertLLMAuditLog(t, c, now.AddDate(0, 0, -1))
	oldRevocation := now.AddDate(0, 0, -100)
	insertAPIKey(t, c, &oldRevocation)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go c.runRetentionCleanup(ctx, 90, 30)

	// Wait until the cleanup has deleted old logs, or time out.
	deadline := time.Now().Add(2 * time.Second)
	var got int64
	for {
		got = countAuditLogs(t, c)
		if got == 1 && countLLMAuditLogs(t, c) == 1 && countAPIKeys(t, c) == 0 {
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

func TestRunRetentionCleanupDisabled(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertAuditLog(t, c, now.AddDate(0, 0, -100))
	insertAuditLog(t, c, now.AddDate(0, 0, -1))

	// Both retention periods disabled means the function returns immediately.
	// Call synchronously — if it ever blocks, the test timeout will catch it.
	c.runRetentionCleanup(t.Context(), 0, 0)

	if got := countAuditLogs(t, c); got != 2 {
		t.Errorf("expected 2 audit logs (cleanup disabled), got %d", got)
	}
}

func TestRunRetentionCleanupRepeats(t *testing.T) {
	c := newTestClient(t)
	old := time.Now().UTC().AddDate(0, 0, -100)
	insertAuditLog(t, c, old)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go c.runRetentionCleanup(ctx, 90, 30)

	waitUntilEmpty := func(stage string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for countAuditLogs(t, c) != 0 {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s cleanup", stage)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitUntilEmpty("initial")

	insertAuditLog(t, c, old)
	waitUntilEmpty("periodic")
}

func TestCleanupRetainedDataSkipsAPIKeysWhenAuditCleanupFails(t *testing.T) {
	c := newTestClient(t)
	now := time.Now().UTC()
	oldRevocation := now.AddDate(0, 0, -100)
	insertAPIKey(t, c, &oldRevocation)

	if err := c.db.WithContext(t.Context()).Migrator().DropTable(&types.MCPAuditLog{}); err != nil {
		t.Fatalf("drop MCP audit log table: %v", err)
	}
	if err := c.cleanupRetainedData(t.Context(), now, 90, 30); err == nil {
		t.Fatal("cleanup succeeded despite missing MCP audit log table")
	}
	if got := countAPIKeys(t, c); got != 1 {
		t.Fatalf("API keys after failed audit cleanup = %d, want 1", got)
	}
}

func TestRunAuditLogCleanupsDoesNotSerializeStreams(t *testing.T) {
	mcpStarted := make(chan struct{})
	releaseMCP := make(chan struct{})
	llmFinished := make(chan struct{})
	cleanupFinished := make(chan error, 1)

	go func() {
		cleanupFinished <- runAuditLogCleanups(
			func() error {
				close(mcpStarted)
				<-releaseMCP
				return nil
			},
			func() error {
				close(llmFinished)
				return nil
			},
		)
	}()

	select {
	case <-mcpStarted:
	case <-time.After(time.Second):
		t.Fatal("MCP audit cleanup did not start")
	}
	select {
	case <-llmFinished:
	case <-time.After(time.Second):
		t.Fatal("LLM audit cleanup was blocked by MCP audit cleanup")
	}
	close(releaseMCP)

	if err := <-cleanupFinished; err != nil {
		t.Fatalf("run audit-log cleanups: %v", err)
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
