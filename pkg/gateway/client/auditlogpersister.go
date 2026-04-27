package client

import (
	"context"
	"errors"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

var log = logger.Package()

func (c *Client) LogMCPAuditEntry(entry types.MCPAuditLog) {
	// Encrypt the audit entry before adding to buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entry.RequestMutated = len(entry.MutatedRequestBody) > 0
	entry.ResponseMutated = len(entry.OriginalResponseBody) > 0

	if err := c.encryptMCPAuditLog(ctx, &entry); err != nil {
		log.Errorf("Failed to encrypt MCP audit log: %v", err)
	}

	c.auditLock.Lock()
	defer c.auditLock.Unlock()

	c.auditBuffer = append(c.auditBuffer, entry)
	if len(c.auditBuffer) >= cap(c.auditBuffer)/2 {
		select {
		case c.kickAuditPersist <- struct{}{}:
		default:
		}
	}
}

func (c *Client) runPersistenceLoop(ctx context.Context, flushInterval time.Duration) {
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.kickAuditPersist:
			timer.Stop()
		case <-timer.C:
		}

		if err := c.persistAuditLogs(); err != nil {
			log.Errorf("Failed to persist audit log: %v", err)
		}

		timer.Reset(flushInterval)
	}
}

func (c *Client) runAuditLogCleanup(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		return
	}

	err := c.deleteOldAuditLogs(ctx, time.Now().UTC(), retentionDays)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("Failed to delete old audit logs: %v", err)
	}

	ticker := time.NewTicker(c.auditLogCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err = c.deleteOldAuditLogs(ctx, time.Now().UTC(), retentionDays)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("Failed to delete old audit logs: %v", err)
			}
		}
	}
}

func (c *Client) deleteOldAuditLogs(ctx context.Context, now time.Time, retentionDays int) error {
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
			"DELETE FROM mcp_audit_logs WHERE id IN (SELECT id FROM mcp_audit_logs WHERE created_at < ? LIMIT ?)",
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

func (c *Client) persistAuditLogs() error {
	c.auditLock.Lock()
	if len(c.auditBuffer) == 0 {
		c.auditLock.Unlock()
		return nil
	}

	buf := c.auditBuffer
	c.auditBuffer = make([]types.MCPAuditLog, 0, cap(c.auditBuffer))
	c.auditLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.insertMCPAuditLogs(ctx, buf); err != nil {
		c.auditLock.Lock()
		c.auditBuffer = append(buf, c.auditBuffer...)
		c.auditLock.Unlock()
		return err
	}

	return nil
}
