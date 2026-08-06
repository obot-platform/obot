package mcp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/tunnel"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHTTPClientForServer(t *testing.T) {
	const timeout = 3 * time.Second

	t.Run("direct server", func(t *testing.T) {
		httpClient, err := (&SessionManager{}).HTTPClientForServer(ServerConfig{}, nil, nil, timeout)
		if err != nil {
			t.Fatal(err)
		}
		if httpClient.Timeout != timeout {
			t.Fatalf("client timeout = %v, want %v", httpClient.Timeout, timeout)
		}
		if httpClient.Transport == nil {
			t.Fatal("direct client uses the default transport")
		}
	})

	t.Run("direct server options", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.Header.Get("X-MCP-Test") != "injected" {
				t.Errorf("injected header = %q, want injected", req.Header.Get("X-MCP-Test"))
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		serverURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		httpClient, err := (&SessionManager{}).HTTPClientForServer(
			ServerConfig{},
			[]string{serverURL.Host},
			http.Header{"X-MCP-Test": {"injected"}},
			timeout,
		)
		if err != nil {
			t.Fatal(err)
		}

		response, err := httpClient.Get(server.URL)
		if err != nil {
			t.Fatalf("allow-listed direct request failed: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusNoContent)
		}
	})

	t.Run("tunnel manager required", func(t *testing.T) {
		_, err := (&SessionManager{}).HTTPClientForServer(ServerConfig{TunnelName: "office"}, nil, nil, timeout)
		if err == nil {
			t.Fatal("tunneled server without tunnel manager returned no error")
		}
	})

	t.Run("tunneled server", func(t *testing.T) {
		tunnelManager, err := tunnel.NewManager(t.Context(), "http://127.0.0.1:8080", fake.NewClientBuilder().Build(), tunnel.PeerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		defer tunnelManager.Close()

		httpClient, err := (&SessionManager{tunnelManager: tunnelManager}).HTTPClientForServer(
			ServerConfig{TunnelName: "office"},
			nil,
			nil,
			timeout,
		)
		if err != nil {
			t.Fatal(err)
		}
		if httpClient.Timeout != timeout {
			t.Fatalf("client timeout = %v, want %v", httpClient.Timeout, timeout)
		}
		if httpClient.Transport == nil {
			t.Fatal("tunneled client uses the default transport")
		}
	})

	t.Run("invalid tunnel name", func(t *testing.T) {
		tunnelManager, err := tunnel.NewManager(t.Context(), "http://127.0.0.1:8080", fake.NewClientBuilder().Build(), tunnel.PeerConfig{})
		if err != nil {
			t.Fatal(err)
		}
		defer tunnelManager.Close()

		_, err = (&SessionManager{tunnelManager: tunnelManager}).HTTPClientForServer(
			ServerConfig{TunnelName: "Office"},
			nil,
			nil,
			timeout,
		)
		if err == nil {
			t.Fatal("invalid tunnel name returned no error")
		}
	})
}
