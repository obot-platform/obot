package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestComputeLockAdmitsOneCallerAtATime(t *testing.T) {
	c := newTestClient(t)

	var holders, most atomic.Int32
	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() {
			errs <- c.WithComputeLock(t.Context(), func() error {
				if n := holders.Add(1); n > most.Load() {
					most.Store(n)
				}
				defer holders.Add(-1)
				// Read through the pool, as counting compute does. SQLite has one connection, so a lock that
				// held it would hang here.
				_, err := c.UserCount(t.Context())
				return err
			})
		}()
	}
	for range callers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if most.Load() != 1 {
		t.Fatalf("%d callers held the lock at once, want 1", most.Load())
	}
}

func TestComputeLockWaiterGivesUpWithItsRequest(t *testing.T) {
	c := newTestClient(t)
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.WithComputeLock(t.Context(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := c.WithComputeLock(ctx, func() error {
		t.Error("a canceled waiter must not run")
		return nil
	})
	if err != context.DeadlineExceeded {
		t.Fatalf("WithComputeLock() = %v, want the waiter's deadline", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := c.WithComputeLock(t.Context(), func() error { return nil }); err != nil {
		t.Fatalf("the lock isn't free again: %v", err)
	}
}
