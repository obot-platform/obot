package localauth

import (
	"testing"
	"time"
)

func TestThrottleBlocksAfterMaxAttempts(t *testing.T) {
	th := newThrottle()

	for range maxFailedAttempts {
		if th.blocked("user@example.com") {
			t.Fatal("blocked before reaching the attempt limit")
		}
		th.failed("user@example.com")
	}

	if !th.blocked("user@example.com") {
		t.Fatal("expected the email to be blocked after the attempt limit")
	}

	// A successful login clears the block.
	th.succeeded("user@example.com")
	if th.blocked("user@example.com") {
		t.Fatal("expected the block to clear after a successful login")
	}
}

func TestThrottleSweepDropsExpiredEntries(t *testing.T) {
	th := newThrottle()

	// Simulate one failure per distinct email, all outside the failure window.
	for _, email := range []string{"a@example.com", "b@example.com", "c@example.com"} {
		th.failures[email] = &failureCount{count: 1, first: time.Now().Add(-2 * failureWindow)}
	}
	// And one recent failure that must survive the sweep.
	th.failed("fresh@example.com")

	th.sweep()

	if len(th.failures) != 1 {
		t.Fatalf("expected sweep to leave only the fresh entry, got %d entries", len(th.failures))
	}
	if _, ok := th.failures["fresh@example.com"]; !ok {
		t.Fatal("sweep removed the in-window entry")
	}
}
