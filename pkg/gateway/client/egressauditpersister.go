package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// LogEgressAuditEntry queues a completed capture or final transport status.
func (c *Client) LogEgressAuditEntry(entry types.EgressAuditLog) {
	c.egressAuditLock.Lock()
	defer c.egressAuditLock.Unlock()

	c.egressAuditBuffer = append(c.egressAuditBuffer, entry)
	if len(c.egressAuditBuffer) >= max(1, c.egressAuditBatchSize) {
		select {
		case c.kickEgressAuditPersist <- struct{}{}:
		default:
		}
	}
}

func (c *Client) runEgressAuditPersistenceLoop(ctx context.Context, interval time.Duration) {
	timer := time.NewTimer(interval)
	defer timer.Stop()

	kick := c.kickEgressAuditPersist

	for {
		select {
		case <-ctx.Done():
			return
		case <-kick:
			timer.Stop()
		case <-timer.C:
		}

		if err := c.writeAuditLogs(ctx, c.flushEgressAuditLogs); err != nil {
			slog.Error("Failed to persist egress audit logs", "error", err)
			// Pending kicks and new arrivals must not bypass the retry interval.
			kick = nil
		} else {
			kick = c.kickEgressAuditPersist
		}

		timer.Reset(interval)
	}
}

// FlushEgressAuditLogs drains queued captures using an independent write context.
func (c *Client) FlushEgressAuditLogs() error {
	return c.writeAuditLogs(context.Background(), c.flushEgressAuditLogs)
}

func (c *Client) flushEgressAuditLogs(ctx context.Context) error {
	c.egressAuditPersistLock.Lock()
	defer c.egressAuditPersistLock.Unlock()

	c.egressAuditLock.Lock()
	if err := ctx.Err(); err != nil {
		c.egressAuditLock.Unlock()
		return err
	}

	if len(c.egressAuditBuffer) == 0 {
		c.egressAuditLock.Unlock()
		return nil
	}

	// Detach only this snapshot so producers can enqueue during database writes.
	buf := c.egressAuditBuffer
	c.egressAuditBuffer = nil
	c.egressAuditLock.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for len(buf) > 0 {
		batchSize := min(len(buf), max(1, c.egressAuditBatchSize))
		if err := c.insertEgressAuditLogs(ctx, buf[:batchSize]); err != nil {
			c.egressAuditLock.Lock()
			// Committed chunks stay durable; retry only the remainder before new arrivals.
			c.egressAuditBuffer = append(buf, c.egressAuditBuffer...)
			c.egressAuditLock.Unlock()

			return err
		}

		// Release committed payloads even while later chunks are blocked or requeued.
		clear(buf[:batchSize])
		buf = buf[batchSize:]
	}

	return nil
}
