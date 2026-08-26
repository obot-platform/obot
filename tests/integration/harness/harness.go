//go:build integration

// Package harness provides a minimal HTTP client for integration tests against
// the isolated obot server started by the integration package's TestMain.
package harness

import (
	"net/http"
	"os"
	"slices"
	"testing"

	"uuid"
)

// Harness is the entry point for an integration test. It holds the base URL of
// the obot server under test, an HTTP client, and a per-test run ID that is
// used to namespace created resources so concurrent runs do not collide.
type Harness struct {
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
		BaseURL: url,
		RunID:   uuid.New().String(),
		// Stream requests set their own deadline; ordinary requests set one per call.
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
	for _, v := range slices.Backward(h.cleanups) {
		v()
	}
}
