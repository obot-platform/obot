package registry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestGuessMimeTypeDoesNotFetchLocalAddresses(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
	}))
	defer server.Close()

	if got := newMimeFetcher().guessMimeType(t.Context(), server.URL+"/icon"); got != "" {
		t.Fatalf("expected no MIME type for a loopback icon URL, got %q", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("expected the loopback icon URL not to be fetched, got %d requests", hits.Load())
	}
}
