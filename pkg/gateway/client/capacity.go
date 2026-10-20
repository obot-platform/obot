package client

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

const (
	mcpCapacityAdvisoryLockID int64 = 0x6f626f744d435043 // "obotMCPC"
)

// WithMCPCapacityLock runs fn under the hosted MCP server capacity lock, so only one replica at a time counts and
// starts servers. On Postgres it's a transaction-scoped advisory lock, released when the transaction ends. SQLite runs
// a single replica, so the process lock is enough.
func (c *Client) WithMCPCapacityLock(ctx context.Context, fn func() error) error {
	c.mcpCapacityLock.Lock()
	defer c.mcpCapacityLock.Unlock()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", mcpCapacityAdvisoryLockID).Error; err != nil {
				return fmt.Errorf("failed to lock MCP capacity: %w", err)
			}
		}
		return fn()
	})
}
