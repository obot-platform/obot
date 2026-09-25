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
