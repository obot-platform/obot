package client

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// insertScanAt creates a scan with children, overriding the GORM-managed
// CreatedAt so it lands on the requested side of the retention cutoff.
func insertScanAt(t *testing.T, c *Client, createdAt time.Time) types.DeviceScan {
	t.Helper()
	scan := insertScan(t, c, types.DeviceScan{
		SubmittedBy: "user-a",
		DeviceID:    "device-a",
		ScannedAt:   createdAt,
		MCPServers:  []types.DeviceScanMCPServer{{Client: "claude-code", Name: "srv", ConfigHash: "hash"}},
		Skills:      []types.DeviceScanSkill{{Client: "claude-code", Name: "skill"}},
		Plugins:     []types.DeviceScanPlugin{{Client: "claude-code", Name: "plugin"}},
		Files:       []types.DeviceScanFile{{Path: "/a", Content: "x"}},
		Clients:     []types.DeviceScanClient{{Name: "claude-code"}},
	})
	if err := c.db.WithContext(t.Context()).
		Model(&types.DeviceScan{}).
		Where("id = ?", scan.ID).
		Update("created_at", createdAt).Error; err != nil {
		t.Fatalf("failed to backdate scan: %v", err)
	}
	scan.CreatedAt = createdAt
	return scan
}

func countScans(t *testing.T, c *Client) int64 {
	t.Helper()
	var count int64
	if err := c.db.WithContext(t.Context()).Model(&types.DeviceScan{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to count device scans: %v", err)
	}
	return count
}

func TestDeleteOldDeviceScans(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	now := time.Now().UTC()
	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -90)

	insertScanAt(t, c, now.AddDate(0, 0, -100))  // old - should be deleted
	insertScanAt(t, c, cutoff.Add(-time.Second)) // one second before cutoff - should be deleted
	insertScanAt(t, c, cutoff)                   // exactly at cutoff boundary - should be kept (< not <=)
	insertScanAt(t, c, now.AddDate(0, 0, -89))   // recent - should be kept
	insertScanAt(t, c, now)                      // recent - should be kept

	if err := c.deleteOldDeviceScans(ctx, now, 90); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countScans(t, c); got != 3 {
		t.Errorf("expected 3 device scans after cleanup, got %d", got)
	}
}

func TestDeleteOldDeviceScansDisabled(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()

	now := time.Now().UTC()
	insertScanAt(t, c, now.AddDate(0, 0, -200))
	insertScanAt(t, c, now.AddDate(0, 0, -100))

	// retentionDays=0 should be a no-op
	if err := c.deleteOldDeviceScans(ctx, now, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countScans(t, c); got != 2 {
		t.Errorf("expected 2 device scans (cleanup disabled), got %d", got)
	}
}

func TestDeleteOldDeviceScansBatching(t *testing.T) {
	c := newTestClient(t) // deviceScanDeleteBatchSize = 3
	ctx := t.Context()

	now := time.Now().UTC()
	// Insert 7 old scans (requires 3 batches: 3+3+1) and 2 recent ones.
	for range 7 {
		insertScanAt(t, c, now.AddDate(0, 0, -100))
	}
	insertScanAt(t, c, now.AddDate(0, 0, -1))
	insertScanAt(t, c, now)

	if err := c.deleteOldDeviceScans(ctx, now, 90); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := countScans(t, c); got != 2 {
		t.Errorf("expected 2 device scans after batched cleanup, got %d", got)
	}
}

func TestRunDeviceScanCleanup(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertScanAt(t, c, now.AddDate(0, 0, -100)) // old
	insertScanAt(t, c, now.AddDate(0, 0, -1))   // recent

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go c.runDeviceScanCleanup(ctx, 90)

	// Wait until the cleanup has deleted old scans, or time out.
	deadline := time.Now().Add(2 * time.Second)
	var got int64
	for {
		got = countScans(t, c)
		if got == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for device scan cleanup, got %d scans", got)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	if got != 1 {
		t.Errorf("expected 1 device scan after cleanup loop, got %d", got)
	}
}

func TestRunDeviceScanCleanupDisabled(t *testing.T) {
	c := newTestClient(t)

	now := time.Now().UTC()
	insertScanAt(t, c, now.AddDate(0, 0, -100))
	insertScanAt(t, c, now.AddDate(0, 0, -1))

	// retentionDays=0 means the function returns immediately without cleanup.
	// Call synchronously — if it ever blocks, the test timeout will catch it.
	c.runDeviceScanCleanup(t.Context(), 0)

	if got := countScans(t, c); got != 2 {
		t.Errorf("expected 2 device scans (cleanup disabled), got %d", got)
	}
}
