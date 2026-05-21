package system

import "testing"

func TestSystemMCPServerAccessPolicies(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		external  bool
		proxyable bool
		webhook   bool
		systemMCP bool
	}{
		{
			name:      "obot mcp server",
			id:        ObotMCPServerName,
			external:  true,
			proxyable: true,
			systemMCP: true,
		},
		{
			name:      "webhook validation system server",
			id:        SystemMCPServerPrefix + MCPWebhookValidationPrefix + "test",
			proxyable: true,
			webhook:   true,
			systemMCP: true,
		},
		{
			name:      "other system server",
			id:        SystemMCPServerPrefix + "other",
			systemMCP: true,
		},
		{
			name: "regular mcp server",
			id:   MCPServerPrefix + "regular",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSystemMCPServerID(tt.id); got != tt.systemMCP {
				t.Fatalf("IsSystemMCPServerID(%q) = %v, want %v", tt.id, got, tt.systemMCP)
			}
			if got := IsWebhookSystemMCPServerID(tt.id); got != tt.webhook {
				t.Fatalf("IsWebhookSystemMCPServerID(%q) = %v, want %v", tt.id, got, tt.webhook)
			}
			if got := IsExternallyAccessibleSystemMCPServerID(tt.id); got != tt.external {
				t.Fatalf("IsExternallyAccessibleSystemMCPServerID(%q) = %v, want %v", tt.id, got, tt.external)
			}
			if got := IsProxyableSystemMCPServerID(tt.id); got != tt.proxyable {
				t.Fatalf("IsProxyableSystemMCPServerID(%q) = %v, want %v", tt.id, got, tt.proxyable)
			}
		})
	}
}
