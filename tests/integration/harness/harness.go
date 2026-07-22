//go:build integration

// Package harness provides a minimal HTTP client for integration tests against
// the isolated obot server started by the integration package's TestMain.
package harness

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"testing"
)

// Harness is the entry point for an integration test. It holds the base URL of
// the obot server under test, an HTTP client, and a per-test run ID that is
// used to namespace created resources so concurrent runs do not collide.
type Harness struct {
	T       *testing.T
	BaseURL string
	RunID   string
	HTTP    *http.Client

	cleanups []func()
}

// New returns a Harness pointed at the isolated integration server.
func New(t *testing.T) *Harness {
	t.Helper()

	url := os.Getenv("OBOT_INTEGRATION_BASE_URL")
	if url == "" {
		t.Fatal("integration server URL was not configured by TestMain")
	}

	h := &Harness{
		T:       t,
		BaseURL: url,
		RunID:   newRunID(),
		// No client-wide timeout: callers pass a context whose deadline governs
		// each request. Launch can block for minutes while the container pulls
		// and becomes healthy.
		HTTP: &http.Client{},
	}

	t.Cleanup(h.runCleanups)
	return h
}

// AddCleanup registers a function to run when the test ends. Cleanups run in
// reverse order of registration (LIFO), like defer.
func (h *Harness) AddCleanup(fn func()) {
	h.cleanups = append(h.cleanups, fn)
}

func (h *Harness) runCleanups() {
	for i := len(h.cleanups) - 1; i >= 0; i-- {
		h.cleanups[i]()
	}
}

func newRunID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
