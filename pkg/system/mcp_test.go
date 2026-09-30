package system

import (
	"testing"
)

func TestMCPConnectURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		id        string
		want      string
	}{
		{
			name:      "server URL without trailing slash",
			serverURL: "https://obot.example.com",
			id:        "server-id",
			want:      "https://obot.example.com/mcp-connect/server-id",
		},
		{
			name:      "server URL with trailing slash",
			serverURL: "https://obot.example.com/",
			id:        "server-id",
			want:      "https://obot.example.com/mcp-connect/server-id",
		},
		{
			name:      "extra separator slashes",
			serverURL: "https://obot.example.com///",
			id:        "/server-id",
			want:      "https://obot.example.com/mcp-connect/server-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MCPConnectURL(tt.serverURL, tt.id); got != tt.want {
				t.Fatalf("MCPConnectURL(%q, %q) = %q, want %q", tt.serverURL, tt.id, got, tt.want)
			}
		})
	}
}

// Obot's loopback requests to itself must target its own listener. The public
// hostname can name a port, such as a Service's port 80, that only exists
// outside Obot's host.
func TestLocalServerURL(t *testing.T) {
	tests := []struct {
		name           string
		httpListenPort int
		want           string
	}{
		{
			name:           "default listen port",
			httpListenPort: 8080,
			want:           "http://localhost:8080",
		},
		{
			name:           "custom listen port",
			httpListenPort: 9090,
			want:           "http://localhost:9090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LocalServerURL(tt.httpListenPort); got != tt.want {
				t.Fatalf("LocalServerURL(%d) = %q, want %q", tt.httpListenPort, got, tt.want)
			}
		})
	}
}
