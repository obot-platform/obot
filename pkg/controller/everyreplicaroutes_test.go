package controller

import (
	"context"
	"errors"
	"testing"
)

func TestLockedGatewayReloadHoldsTheLock(t *testing.T) {
	locked := false
	withLock := func(_ context.Context, fn func() error) error {
		locked = true
		defer func() { locked = false }()
		return fn()
	}
	var reloadedUnderLock bool
	reload := lockedGatewayReload(withLock, func(revision int64) (bool, error) {
		reloadedUnderLock = locked
		return revision == 7, nil
	})
	reloaded, err := reload(t.Context(), 7)
	if err != nil || !reloaded {
		t.Fatalf("reload() = %v, %v; want true, nil", reloaded, err)
	}
	if !reloadedUnderLock {
		t.Fatal("the gateway reloaded without holding the lock")
	}

	busy := errors.New("busy")
	if _, err := lockedGatewayReload(func(context.Context, func() error) error { return busy }, func(int64) (bool, error) {
		t.Fatal("reloaded without the lock")
		return false, nil
	})(t.Context(), 8); !errors.Is(err, busy) {
		t.Fatalf("expected the lock's error, got %v", err)
	}
}
