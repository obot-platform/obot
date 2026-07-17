package system

import "testing"

func TestMCPConnectURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		path      string
		id        string
		want      string
	}{
		{
			name:      "server URL without trailing slash",
			serverURL: "https://obot.example.com",
			path:      "/mcp-connect/",
			id:        "server-id",
			want:      "https://obot.example.com/mcp-connect/server-id",
		},
		{
			name:      "server URL with trailing slash",
			serverURL: "https://obot.example.com/",
			path:      "/mcp-connect/",
			id:        "server-id",
			want:      "https://obot.example.com/mcp-connect/server-id",
		},
		{
			name:      "extra separator slashes",
			serverURL: "https://obot.example.com///",
			path:      "//mcp-connect-composite//",
			id:        "/server-id",
			want:      "https://obot.example.com/mcp-connect-composite/server-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpConnectURL(tt.serverURL, tt.path, tt.id); got != tt.want {
				t.Fatalf("mcpConnectURL(%q, %q, %q) = %q, want %q", tt.serverURL, tt.path, tt.id, got, tt.want)
			}
		})
	}
}
