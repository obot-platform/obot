package client

import (
	"context"
	"errors"
	"time"

	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"gorm.io/gorm"
)

func (c *Client) runLLMAuditLogCleanup(ctx context.Context, retentionDays int) {
	err := c.maintainLLMAuditLogPartitions(ctx, time.Now().UTC(), retentionDays)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("Failed to maintain LLM audit log partitions: %v", err)
	}

	ticker := time.NewTicker(c.auditLogCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			err = c.maintainLLMAuditLogPartitions(ctx, now.UTC(), retentionDays)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("Failed to maintain LLM audit log partitions: %v", err)
			}
		}
	}
}

func (c *Client) maintainLLMAuditLogPartitions(ctx context.Context, now time.Time, retentionDays int) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return gatewaydb.MaintainLLMAuditLogPartitions(ctx, tx, retentionDays, now)
	})
}
