package mcp

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// Tenant identity can be carried by a connection header even when the URL is unchanged.
func TestOAuthTargetHashIncludesConnectionHeaders(t *testing.T) {
	server := v1.MCPServer{Name: "component"}
	before := ServerConfig{
		URL:                     "https://upstream.test/mcp",
		MCPServerInstanceID:     "mcpsi1connection",
		PassthroughHeaderNames:  []string{"X-Tenant"},
		PassthroughHeaderValues: []string{"tenant-a"},
	}
	after := before
	after.PassthroughHeaderValues = []string{"tenant-b"}
	if OAuthTargetHash(server, before) == OAuthTargetHash(server, after) {
		t.Fatal("tenant changed on the same connection, but captured target still validates")
	}

	after = before
	after.PassthroughHeaderNames = []string{"X-Account"}
	if OAuthTargetHash(server, before) == OAuthTargetHash(server, after) {
		t.Fatal("connection header name changed, but captured target still validates")
	}
}
