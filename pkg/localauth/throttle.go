package localauth

import (
	"context"
	"strings"
	"sync"
	"time"
)

const (
	maxFailedAttempts = 10
	failureWindow     = 15 * time.Minute
)

// dummyHash is verified when a login is attempted for an email that has no local user, so that
// the response time doesn't reveal whether the account exists. It is the hash of a password that
// cannot be entered, since it is longer than any form field allows.
var dummyHash = mustHash(strings.Repeat("x", 128))

func mustHash(password string) string {
	h, err := HashPassword(password)
	if err != nil {
		panic(err)
	}
	return h
}

// throttle slows down password guessing by locking an email out after too many failures.
// It is per-process and in-memory: it is a speed bump, not a distributed rate limiter.
type throttle struct {
	lock     sync.Mutex
	failures map[string]*failureCount
}

type failureCount struct {
	count int
	first time.Time
}

func newThrottle() *throttle {
	return &throttle{
		failures: map[string]*failureCount{},
	}
}

func (t *throttle) blocked(email string) bool {
	t.lock.Lock()
	defer t.lock.Unlock()

	f := t.failures[email]
	if f == nil {
		return false
	}

	if time.Since(f.first) > failureWindow {
		delete(t.failures, email)
		return false
	}

	return f.count >= maxFailedAttempts
}

func (t *throttle) failed(email string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	f := t.failures[email]
	if f == nil || time.Since(f.first) > failureWindow {
		t.failures[email] = &failureCount{count: 1, first: time.Now()}
		return
	}

	f.count++
}

func (t *throttle) succeeded(email string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	delete(t.failures, email)
}

// sweep drops entries whose failure window has elapsed. Without it the map would grow one entry
// per distinct failing email and only ever shrink when that exact email is retried or succeeds,
// so an enumeration attempt (many unique emails) would leak memory unbounded.
func (t *throttle) sweep() {
	t.lock.Lock()
	defer t.lock.Unlock()

	for email, f := range t.failures {
		if time.Since(f.first) > failureWindow {
			delete(t.failures, email)
		}
	}
}

// run periodically sweeps expired entries until the context is cancelled.
func (t *throttle) run(ctx context.Context) {
	ticker := time.NewTicker(failureWindow)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.sweep()
		case <-ctx.Done():
			return
		}
	}
}
