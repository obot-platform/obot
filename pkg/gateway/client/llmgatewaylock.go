package client

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	// llmGatewayLockID is the Postgres advisory lock that serializes changes Obot itself makes
	// to the LLM gateway's shared configuration across replicas.
	llmGatewayLockID int64 = 0x6f626f744c4c4d67 // "obotLLMg"
)

var (
	// llmGatewayLockWait bounds how long a replica waits to take the LLM gateway lock.
	llmGatewayLockWait = 5 * time.Minute

	// errLLMGatewayLockBusy is returned when another replica holds the LLM gateway lock for
	// longer than llmGatewayLockWait.
	errLLMGatewayLockBusy = errors.New("another Obot replica is changing the LLM gateway configuration; try again")
)

// WithLLMGatewayLock runs fn while holding a lock every Obot replica shares, so replicas that
// start together don't make the same gateway changes at once, each from its own copy of the
// configuration. With SQLite there is a single replica, and fn just runs.
//
// The lock is a session-level advisory lock on a connection of its own, held until fn
// returns, however long that takes: it can't expire while fn is still changing the gateway,
// and no transaction is held open meanwhile. If the replica dies, Postgres releases it with
// the connection. The wait to take it is bounded, and doesn't depend on ctx's cancellation,
// so a caller that goes away (an administrator's request) can't leave a change half-made.
func (c *Client) WithLLMGatewayLock(ctx context.Context, fn func() error) error {
	ctx = context.WithoutCancel(ctx)
	db := c.db.WithContext(ctx)
	if db.Name() != "postgres" {
		return fn()
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get a connection for the LLM gateway lock: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(llmGatewayLockWait)
	for delay := 100 * time.Millisecond; ; delay = min(delay*2, 2*time.Second) {
		var locked bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", llmGatewayLockID).Scan(&locked); err != nil {
			return fmt.Errorf("failed to take the LLM gateway lock: %w", err)
		}
		if locked {
			break
		}
		if time.Now().Add(delay).After(deadline) {
			return errLLMGatewayLockBusy
		}
		time.Sleep(delay)
	}
	defer func() {
		// The lock belongs to this connection's session; closing the connection would release
		// it too, but the pool may keep the session open, so release it explicitly.
		unlockCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", llmGatewayLockID)
	}()
	return fn()
}
