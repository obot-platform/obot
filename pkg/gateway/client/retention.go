package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	// maxAuditLogRetentionDays keeps the cleanup cutoff within the dates the database can store.
	maxAuditLogRetentionDays = 100 * 365
)

func apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) int {
	if mcpAuditLogRetentionDays <= 0 || llmAuditLogRetentionDays <= 0 {
		return 0
	}
	return max(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)
}

// runRetentionCleanup deletes audit logs, revoked API keys and other data past retention, now and every
// auditLogCleanupInterval until ctx is done. Each pass reads the audit log retention again.
func (c *Client) runRetentionCleanup(ctx context.Context, entitlements EntitlementProvider) {
	run := func(now time.Time) {
		mcpDays, llmDays := c.auditLogRetention(ctx, entitlements)
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

// auditLogRetention returns how many days of MCP and LLM audit logs to keep. 0 keeps them all. An audit log retention
// in the entitlements replaces the configured days. If the entitlements can't be read, every audit log is kept.
func (c *Client) auditLogRetention(ctx context.Context, entitlements EntitlementProvider) (mcpDays, llmDays int) {
	granted, err := entitlements.Entitlements(ctx)
	if err != nil {
		slog.Error("Failed to read audit log retention, keeping audit logs", "error", err)
		return 0, 0
	}
	if days := granted.AuditLogRetentionDays; days != nil && *days > 0 {
		days := int(min(*days, maxAuditLogRetentionDays))
		return days, days
	}
	return c.auditLogRetentionDays, c.llmAuditLogRetentionDays
}

func (c *Client) cleanupRetainedData(ctx context.Context, now time.Time, mcpAuditLogRetentionDays, llmAuditLogRetentionDays int) error {
	// Short-lived runner keys do not need to wait for audit-log retention.
	var expiredKeyErr error
	if err := c.deleteUnreferencedAPIKeys(ctx, "claude_environment = ? AND expires_at <= ?", true, now); err != nil {
		// Credential failures must not prevent independent audit-log cleanup.
		expiredKeyErr = fmt.Errorf("delete expired Claude API keys: %w", err)
	}

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
		return errors.Join(expiredKeyErr, err)
	}

	if err := c.deleteOldRevokedAPIKeys(ctx, now, apiKeyRetentionDays(mcpAuditLogRetentionDays, llmAuditLogRetentionDays)); err != nil {
		return errors.Join(expiredKeyErr, fmt.Errorf("delete old revoked API keys: %w", err))
	}
	if err := c.deleteOldLLMGatewayBudgetUsage(ctx, now); err != nil {
		return errors.Join(expiredKeyErr, fmt.Errorf("delete old LLM gateway budget usage: %w", err))
	}
	return expiredKeyErr
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
	return c.deleteUnreferencedAPIKeys(ctx, "revoked_at IS NOT NULL AND revoked_at < ?", cutoff)
}

// Both cleanup paths preserve the current revoked key as the session's revocation marker.
func (c *Client) deleteUnreferencedAPIKeys(ctx context.Context, condition string, args ...any) error {
	var afterID uint
	var referenced map[uint]bool
	for {
		var keys []types.APIKey
		query := c.db.WithContext(ctx).Where(condition, args...)
		if err := query.Select("id", "claude_environment", "revoked_at").
			Where("id > ?", afterID).Order("id").Limit(c.auditLogDeleteBatchSize).Find(&keys).Error; err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}

		if referenced == nil {
			var credentials []types.Credential
			if err := c.db.WithContext(ctx).Where("context IN ?", []string{"substrate-agent", ClaudeLLMCredentialContext}).Find(&credentials).Error; err != nil {
				return err
			}
			referenced = make(map[uint]bool, len(credentials))
			for _, credential := range credentials {
				if err := c.decryptCredential(ctx, &credential); err != nil {
					return err
				}
				keyID, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
				if err != nil {
					return fmt.Errorf("read Claude credential key ID: %w", err)
				}
				referenced[uint(keyID)] = true
			}
		}

		var ids, guardedIDs []uint
		for _, key := range keys {
			if key.ClaudeEnvironment && referenced[key.ID] {
				if key.RevokedAt != nil {
					continue
				}
				guardedIDs = append(guardedIDs, key.ID)
			}
			ids = append(ids, key.ID)
		}
		if len(ids) > 0 {
			deleteQuery := c.db.WithContext(ctx).Where(condition, args...).Where("id IN ?", ids)
			if len(guardedIDs) > 0 {
				// A referenced key may have been revoked since the batch was read.
				deleteQuery = deleteQuery.Where("revoked_at IS NULL OR id NOT IN ?", guardedIDs)
			}
			if err := deleteQuery.Delete(&types.APIKey{}).Error; err != nil {
				return err
			}
		}
		// Advance past retained keys too, so a full protected batch cannot stall cleanup.
		afterID = keys[len(keys)-1].ID
	}
}
