package client

import (
	"context"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

var log = logger.Package()

func (c *Client) LogMCPAuditEntry(entry types.MCPAuditLog) {
	// Encrypt the audit entry before adding to buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
