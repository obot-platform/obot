package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// maxRetentionDays caps licensed retention, so its cutoff stays within the dates the database can store.
	maxRetentionDays = 100 * 365
)

// RetentionLimits supplies licensed limits on how long the gateway keeps data.
type RetentionLimits interface {
	// AuditLogRetentionLimit returns the number of days MCP and LLM audit logs are kept.
	AuditLogRetentionLimit(context.Context) (SystemLimit, error)
}

func apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) int {
	if mcpAuditLogRetentionDays <= 0 || llmAuditLogRetentionDays <= 0 {
		return 0
	}
	return max(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)
}

// runAuditCleanup deletes expired audit logs and revoked API keys now, and again every cleanup interval.
// A licensed retention limit replaces the configured retention, and is looked up again on every pass.
func (c *Client) runAuditCleanup(ctx context.Context, limits RetentionLimits) {
	// Keep everything until a lookup succeeds, so failing at startup deletes nothing.
	var mcpDays, llmDays int
	run := func(now time.Time) {
		if limit, err := limits.AuditLogRetentionLimit(ctx); err != nil {
			// Keep the retention from the last pass, so a transient failure doesn't change it.
			slog.Error("Failed to resolve audit log retention limit, keeping the previous retention", "error", err)
		} else if !limit.Unlimited && limit.Maximum > 0 {
			// A licensed limit replaces the configured retention.
			mcpDays = int(min(limit.Maximum, maxRetentionDays))
			llmDays = mcpDays
		} else {
			// No licensed limit, use the configured retention.
			mcpDays, llmDays = c.mcpAuditLogRetentionDays, c.llmAuditLogRetentionDays
		}

		if mcpDays <= 0 && llmDays <= 0 {
			// Nothing to clean up this pass. Keep the loop running, a license may add a limit later.
			return
		}

		if err := c.cleanupRetainedData(ctx, now.UTC(), mcpDays, llmDays); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Failed to clean up retained gateway data", "error", err)
		}
	}
	run(time.Now())

	ticker := time.NewTicker(c.auditLogCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			run(now)
		}
	}
}

func (c *Client) cleanupRetainedData(ctx context.Context, now time.Time, mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) error {
	if err := runAuditLogCleanups(
		func() error {
			if err := c.deleteOldMCPAuditLogs(ctx, now, mcpAuditLogRetentionDays); err != nil {
				return fmt.Errorf("delete old MCP audit logs: %w", err)
			}
			return nil
		},
		func() error {
			if err := c.deleteOldLLMAuditLogs(ctx, now, llmAuditLogRetentionDays); err != nil {
				return fmt.Errorf("delete old LLM audit logs: %w", err)
			}
			return nil
		},
	); err != nil {
		return err
	}

	if err := c.deleteOldRevokedAPIKeys(ctx, now, apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)); err != nil {
		return fmt.Errorf("delete old revoked API keys: %w", err)
	}
	return nil
}

func runAuditLogCleanups(mcpCleanup, llmCleanup func() error) error {
	var (
		waitGroup sync.WaitGroup
		mcpErr    error
		llmErr    error
	)
	waitGroup.Go(func() {
		mcpErr = mcpCleanup()
	})
	waitGroup.Go(func() {
		llmErr = llmCleanup()
	})
	waitGroup.Wait()
	return errors.Join(mcpErr, llmErr)
}

func (c *Client) deleteOldRevokedAPIKeys(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)
	for {
		result := c.db.WithContext(ctx).Exec(
			"DELETE FROM api_keys WHERE id IN (SELECT id FROM api_keys WHERE revoked_at IS NOT NULL AND revoked_at < ? LIMIT ?)",
			cutoff, c.auditLogDeleteBatchSize,
		)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < int64(c.auditLogDeleteBatchSize) {
			return nil
		}
	}
}
