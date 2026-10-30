package client

import (
	"context"

	"github.com/obot-platform/obot/pkg/gateway/db"
	"gorm.io/gorm"
)

// WithLock runs fn in a transaction that holds the lock until it commits or rolls back. The transaction holds a
// database connection while it waits for the lock, and fn's queries use others, so callers in this process take turns
// first: only one caller per replica holds or waits for a lock, and waiters don't fill the pool. That also serializes
// SQLite, which has no lock to take.
func (c *Client) WithLock(ctx context.Context, id db.LockID, fn func(tx *gorm.DB) error) error {
	gate := c.lockGate(id)
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-gate }()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := db.Lock(tx, id); err != nil {
			return err
		}
		return fn(tx)
	})
}

// lockGate returns the channel that admits one caller in this process to the lock.
func (c *Client) lockGate(id db.LockID) chan struct{} {
	gate, ok := c.locks.Load(id)
	if !ok {
		gate, _ = c.locks.LoadOrStore(id, make(chan struct{}, 1))
	}
	return gate.(chan struct{})
}
