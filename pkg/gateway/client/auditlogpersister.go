package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// DisableMCPAuditLog stops collection and persistence of new MCP audit logs,
// including gateway traffic and device-submitted local-agent tool calls. Call it
// before the client starts serving traffic. Existing logs remain readable.
func (c *Client) DisableMCPAuditLog() {
	c.mcpAuditDisabled = true
}

func (c *Client) MCPAuditLogEnabled() bool {
	return c != nil && !c.mcpAuditDisabled
}

func (c *Client) LogMCPAuditEntry(entry types.MCPAuditLog) {
	if !c.MCPAuditLogEnabled() {
		return
	}

	// Encrypt the audit entry before adding to buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entry.NormalizeMCPFields()
	if err := entry.ValidateSourceFields(); err != nil {
		slog.Error("Invalid MCP audit log source fields", "error", err)
		return
	}
	mcp := entry.MCP()
	mcp.RequestMutated = len(mcp.MutatedRequestBody) > 0
	mcp.ResponseMutated = len(mcp.OriginalResponseBody) > 0

	if err := c.encryptMCPAuditLog(ctx, &entry); err != nil {
		slog.Error("Failed to encrypt MCP audit log", "error", err,
			"mcpID", mcp.MCPID,
			"requestID", mcp.RequestID,
			"proxyExchangeID", mcp.ProxyExchangeID,
			"callType", mcp.CallType,
		)
		// Drop the entry so failed or partially encrypted payloads cannot be persisted.
		return
	}

	c.auditLock.Lock()
	defer c.auditLock.Unlock()

	batchSize, _ := auditBatchCapacity(c.auditLogBatchSize)
	c.auditBuffer = append(c.auditBuffer, entry)
	if len(c.auditBuffer) >= batchSize {
		select {
		case c.kickAuditPersist <- struct{}{}:
		default:
		}
	}
}

func (c *Client) runMCPAuditLogPersistenceLoop(ctx context.Context, flushInterval time.Duration) {
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

		if err := c.writeAuditLogs(ctx, c.persistMCPAuditLogsContext); err != nil {
			slog.Error("Failed to persist audit log", "error", err)
		}

		timer.Reset(flushInterval)
	}
}

func (c *Client) deleteOldMCPAuditLogs(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)

	for {
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

func (c *Client) persistMCPAuditLogsContext(ctx context.Context) error {
	// The buffer lock is released during DB work; serialize flushes to preserve batch order.
	c.auditPersistLock.Lock()
	defer c.auditPersistLock.Unlock()

	c.auditLock.Lock()
	if err := ctx.Err(); err != nil {
		c.auditLock.Unlock()
		return err
	}
	if len(c.auditBuffer) == 0 {
		c.auditLock.Unlock()
		return nil
	}

	buf := c.auditBuffer
	c.auditBuffer = make([]types.MCPAuditLog, 0, cap(c.auditBuffer))
	c.auditLock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := c.insertMCPAuditLogs(ctx, buf); err != nil {
		// Keep original ordering while producers continue adding entries.
		c.auditLock.Lock()
		c.auditBuffer = append(buf, c.auditBuffer...)
		c.auditLock.Unlock()
		return err
	}

	return nil
}
