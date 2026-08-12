package client

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) int {
	if mcpAuditLogRetentionDays <= 0 || llmAuditLogRetentionDays <= 0 {
		return 0
	}
	return max(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)
}

func (c *Client) runRetentionCleanup(ctx context.Context, mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) {
	if mcpAuditLogRetentionDays <= 0 && llmAuditLogRetentionDays <= 0 {
		return
	}

	run := func(now time.Time) {
		if err := c.cleanupRetainedData(ctx, now.UTC(), mcpAuditLogRetentionDays, llmAuditLogRetentionDays); err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("Failed to clean up retained gateway data: %v", err)
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
	var auditLogErrors []error
	if err := c.deleteOldMCPAuditLogs(ctx, now, mcpAuditLogRetentionDays); err != nil {
		auditLogErrors = append(auditLogErrors, fmt.Errorf("delete old MCP audit logs: %w", err))
	}
	if err := c.deleteOldLLMAuditLogs(ctx, now, llmAuditLogRetentionDays); err != nil {
		auditLogErrors = append(auditLogErrors, fmt.Errorf("delete old LLM audit logs: %w", err))
	}
	if err := errors.Join(auditLogErrors...); err != nil {
		return err
	}

	if err := c.deleteOldRevokedAPIKeys(ctx, now, apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)); err != nil {
		return fmt.Errorf("delete old revoked API keys: %w", err)
	}
	return nil
}

func (c *Client) deleteOldRevokedAPIKeys(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

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
