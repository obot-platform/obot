package registry

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
)

func TestGuessMimeTypeDoesNotFetchLocalAddressesByDefault(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
	}))
	defer server.Close()

	if got := newMimeFetcher(mcp.RemoteMCPURLValidationConfig{}).guessMimeType(t.Context(), server.URL+"/icon"); got != "" {
		t.Fatalf("expected no MIME type for a loopback icon URL, got %q", got)
	}
	if hits.Load() != 0 {
		t.Fatalf("expected the loopback icon URL not to be fetched, got %d requests", hits.Load())
	}
}

func TestGuessMimeTypeFetchesLocalAddressesWhenAllowed(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
	}))
	defer server.Close()

	fetcher := newMimeFetcher(mcp.RemoteMCPURLValidationConfig{AllowLocalhostMCP: true})
	if got := fetcher.guessMimeType(t.Context(), server.URL+"/icon"); got != "image/png" {
		t.Fatalf("expected image/png for an allowed loopback icon URL, got %q", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected the allowed loopback icon URL to be fetched once, got %d requests", hits.Load())
	}
}
