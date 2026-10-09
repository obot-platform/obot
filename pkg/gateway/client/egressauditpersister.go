package client

import (
	"context"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	egressAuditMaxEntries = 256
	egressAuditMaxBytes   = 64 << 20
)

// Include payload capacities and metadata so small entries cannot bypass the bound.
func egressAuditEntryBytes(entry types.EgressAuditLog) int {
	size := 1024 + cap(entry.RequestBody) + cap(entry.ResponseBody) + cap(entry.RequestHeaders) + cap(entry.ResponseHeaders)
	for _, value := range []string{entry.ID, entry.UserID, entry.ClaudeAccount, entry.EnvironmentID, entry.ClaudeSession, entry.Actor, entry.Method, entry.Scheme, entry.Host, entry.Path, entry.Outcome} {
		size += len(value)
	}
	return size
}

// LogEgressAuditEntry queues a completed capture or final transport status.
func (c *Client) LogEgressAuditEntry(entry types.EgressAuditLog) {
	c.egressAuditLock.Lock()
	defer c.egressAuditLock.Unlock()

	size := egressAuditEntryBytes(entry)
	// Reservations include the in-flight batch, so a failed flush can always requeue.
	if c.egressAuditPending >= egressAuditMaxEntries || size > egressAuditMaxBytes-c.egressAuditBytes {
		c.egressAuditDropped++
		if time.Since(c.egressAuditOverflowLogged) >= 30*time.Second {
			slog.Error("Egress audit queue full; dropping new entries", "dropped_total", c.egressAuditDropped)
			c.egressAuditOverflowLogged = time.Now()
		}
		return
	}

	c.egressAuditPending++
	c.egressAuditBytes += size
	c.egressAuditBuffer = append(c.egressAuditBuffer, entry)
	if len(c.egressAuditBuffer) >= cap(c.egressAuditBuffer)/2 {
		select {
		case c.kickEgressAuditPersist <- struct{}{}:
		default:
		}
	}
}

func (c *Client) runEgressAuditPersistenceLoop(ctx context.Context, interval time.Duration) {
	defer close(c.egressAuditDone)
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.kickEgressAuditPersist:
			timer.Stop()
		case <-timer.C:
		}

		if err := c.FlushEgressAuditLogs(); err != nil {
			slog.Error("Failed to persist egress audit logs", "error", err)
		}
		timer.Reset(interval)
	}
}

// FlushEgressAuditLogs drains queued captures using an independent write context.
func (c *Client) FlushEgressAuditLogs() error {
	c.egressAuditPersistLock.Lock()
	defer c.egressAuditPersistLock.Unlock()

	c.egressAuditLock.Lock()
	buf := c.egressAuditBuffer
	bytes := c.egressAuditBytes
	c.egressAuditBuffer = make([]types.EgressAuditLog, 0, cap(buf))
	c.egressAuditLock.Unlock()
	if len(buf) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.insertEgressAuditLogs(ctx, buf); err != nil {
		c.egressAuditLock.Lock()
		c.egressAuditBuffer = append(buf, c.egressAuditBuffer...)
		c.egressAuditLock.Unlock()
		return err
	}
	c.egressAuditLock.Lock()
	c.egressAuditPending -= len(buf)
	c.egressAuditBytes -= bytes
	c.egressAuditLock.Unlock()
	return nil
}
