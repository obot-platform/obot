package client

import (
	"context"
	"errors"
	"time"
)

func (c *Client) runDeviceScanCleanup(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		return
	}

	err := c.deleteOldDeviceScans(ctx, time.Now().UTC(), retentionDays)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("Failed to delete old device scans: %v", err)
	}

	ticker := time.NewTicker(c.deviceScanCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			err = c.deleteOldDeviceScans(ctx, now.UTC(), retentionDays)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("Failed to delete old device scans: %v", err)
			}
		}
	}
}

// The scans' child rows go with them via the ON DELETE CASCADE
// constraints on device_scan_*.device_scan_id.
func (c *Client) deleteOldDeviceScans(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)

	for {
		result := c.db.WithContext(ctx).Exec(
			"DELETE FROM device_scans WHERE id IN (SELECT id FROM device_scans WHERE created_at < ? LIMIT ?)",
			cutoff, c.deviceScanDeleteBatchSize,
		)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < int64(c.deviceScanDeleteBatchSize) {
			return nil
		}
	}
}
