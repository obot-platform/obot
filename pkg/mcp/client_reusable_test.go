package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// A cached client whose connection permanently failed must not be reused,
// while one whose session the component reported missing is still recovered.
func TestClientReusableAfterComponentDropsSession(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		wantReusable bool
	}{
		// A plain 400 permanently fails the SDK connection.
		{name: "connection failed", status: http.StatusBadRequest, wantReusable: false},
		// A 404 marks the session missing, which session recovery repairs on a
		// later call even if this call's reconnect failed.
		{name: "session missing", status: http.StatusNotFound, wantReusable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "1"}, nil)
			gomcp.AddTool(server, &gomcp.Tool{Name: "ping"}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
				return &gomcp.CallToolResult{}, nil, nil
			})
			inner := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
			var mu sync.Mutex
			reaped := map[string]bool{}
			unavailable := false
			component := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id := r.Header.Get("Mcp-Session-Id")
				mu.Lock()
				gone, refuse := reaped[id], unavailable && id == ""
				mu.Unlock()
				switch {
				case gone:
					http.Error(w, "session expired", test.status)
				case refuse:
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				default:
					inner.ServeHTTP(w, r)
				}
			}))
			t.Cleanup(component.Close)

			sdk := gomcp.NewClient(&gomcp.Implementation{Name: "obot", Version: "1"}, &gomcp.ClientOptions{Capabilities: &gomcp.ClientCapabilities{}})
			client, err := newRecoveringClient(t.Context(), sdk, gomcp.StreamableClientTransport{Endpoint: component.URL, DisableStandaloneSSE: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			client.jwt = jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))})
			if !client.reusable() {
				t.Fatal("healthy client is not reusable")
			}

			// Expire the session and refuse new ones, so session recovery cannot
			// reconnect within this call and the expired session stays current.
			mu.Lock()
			reaped[client.ID()] = true
			unavailable = true
			mu.Unlock()
			_, _ = client.CallTool(t.Context(), &gomcp.CallToolParams{Name: "ping"})

			// The SDK session ends asynchronously after the failure.
			deadline := time.Now().Add(5 * time.Second)
			for client.reusable() != test.wantReusable && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}
			if got := client.reusable(); got != test.wantReusable {
				t.Fatalf("reusable = %v, want %v", got, test.wantReusable)
			}
		})
	}
}
