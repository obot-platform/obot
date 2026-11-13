package client

import (
	"context"

	"gorm.io/gorm"
)

// The IDs of the locks taken with withAdvisoryLock. They must not change, so replicas running different versions
// during an upgrade take the same lock.
const (
	claudeProvisionAdvisoryLockID int64 = 672046923
	computeAdvisoryLockID         int64 = 0x6f626f744d435043 // "obotMCPC"
)

// withAdvisoryLock runs fn while holding the Postgres advisory lock id, which is held across Obot replicas.
//
// A transaction of its own holds the lock until fn returns. That transaction holds a database connection while it
// waits, and fn's queries use others. Waiting at the gate first keeps a replica's waiters from filling the pool and
// stalling the holder: only one caller per replica holds or waits for the lock.
func (c *Client) withAdvisoryLock(ctx context.Context, id int64, fn func() error) error {
	return c.withGate(ctx, id, func() error {
		return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", id).Error; err != nil {
				return err
			}
			return fn()
		})
	})
}

// withGate runs fn once this process's other callers for the lock id are done, or returns when ctx ends first.
func (c *Client) withGate(ctx context.Context, id int64, fn func() error) error {
	gate, ok := c.lockGates.Load(id)
	if !ok {
		gate, _ = c.lockGates.LoadOrStore(id, make(chan struct{}, 1))
	}
	select {
	case gate.(chan struct{}) <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-gate.(chan struct{}) }()
	return fn()
}

// WithComputeLock runs fn while holding the lock that serializes the writes that change compute, so compute can be
// counted and a write made without another replica's change landing in between. Those writes are made by storage on its
// own connections, not in a gateway transaction, so the lock is held around fn.
//
// SQLite runs a single replica on a single connection, which fn needs, so there the gate is the whole lock.
func (c *Client) WithComputeLock(ctx context.Context, fn func() error) error {
	if c.db.WithContext(ctx).Name() != "postgres" {
		return c.withGate(ctx, computeAdvisoryLockID, fn)
	}
	return c.withAdvisoryLock(ctx, computeAdvisoryLockID, fn)
}
