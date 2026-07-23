package client

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// LogEnforcementDecision buffers the decision row for asynchronous persistence.
// The decision endpoint returns to the device before the row is durably flushed,
// keeping the database off the synchronous decision path (the block/allow
// verdict never waits on a write).
func (c *Client) LogEnforcementDecision(entry types.EnforcementDecisionLog) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	c.enforcementLock.Lock()
	defer c.enforcementLock.Unlock()

	c.enforcementBuffer = append(c.enforcementBuffer, entry)
	if cap(c.enforcementBuffer) > 0 && len(c.enforcementBuffer) >= cap(c.enforcementBuffer)/2 {
		select {
		case c.kickEnforcementPersist <- struct{}{}:
		default:
		}
	}
}

func (c *Client) runEnforcementDecisionPersistenceLoop(ctx context.Context, flushInterval time.Duration) {
	timer := time.NewTimer(flushInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.kickEnforcementPersist:
			timer.Stop()
		case <-timer.C:
		}

		if err := c.persistEnforcementDecisions(); err != nil {
			log.Errorf("Failed to persist enforcement decision log: %v", err)
		}

		timer.Reset(flushInterval)
	}
}

func (c *Client) persistEnforcementDecisions() error {
	c.enforcementLock.Lock()
	if len(c.enforcementBuffer) == 0 {
		c.enforcementLock.Unlock()
		return nil
	}

	buf := c.enforcementBuffer
	c.enforcementBuffer = make([]types.EnforcementDecisionLog, 0, cap(c.enforcementBuffer))
	c.enforcementLock.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.insertEnforcementDecisions(ctx, buf); err != nil {
		// On failure, prepend the batch back so nothing is lost.
		c.enforcementLock.Lock()
		c.enforcementBuffer = append(buf, c.enforcementBuffer...)
		c.enforcementLock.Unlock()
		return err
	}

	return nil
}

func (c *Client) insertEnforcementDecisions(ctx context.Context, logs []types.EnforcementDecisionLog) error {
	if len(logs) == 0 {
		return nil
	}
	for i := range logs {
		logs[i].CreatedAt = logs[i].CreatedAt.UTC()
	}
	if err := c.db.WithContext(ctx).CreateInBatches(logs, 100).Error; err != nil {
		return fmt.Errorf("failed to insert enforcement decision logs: %w", err)
	}
	return nil
}
