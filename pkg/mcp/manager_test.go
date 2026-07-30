package mcp

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/tunnel"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHTTPClientForServer(t *testing.T) {
	const timeout = 3 * time.Second

	t.Run("direct server", func(t *testing.T) {
		httpClient, err := (&SessionManager{}).HTTPClientForServer(ServerConfig{}, timeout)
		if err != nil {
			t.Fatal(err)
		}
		if httpClient.Timeout != timeout {
			t.Fatalf("client timeout = %v, want %v", httpClient.Timeout, timeout)
		}
		if httpClient.Transport != nil {
			t.Fatalf("direct client transport = %#v, want default transport", httpClient.Transport)
		}
	})

	t.Run("tunnel manager required", func(t *testing.T) {
		_, err := (&SessionManager{}).HTTPClientForServer(ServerConfig{TunnelName: "office"}, timeout)
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
			timeout,
		)
		if err == nil {
			t.Fatal("invalid tunnel name returned no error")
		}
	})
}
