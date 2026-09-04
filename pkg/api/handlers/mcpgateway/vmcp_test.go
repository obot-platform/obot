package mcpgateway

import (
	"testing"
)

func TestVMCPCompositeLoopbackURLsPreservesVMCPID(t *testing.T) {
	const (
		serverURL     = "https://obot.example.com/"
		vmcpID        = "vmcp1shared"
		internalBase  = "http://obot.obot-system.svc.cluster.local"
		wantPublicURL = "https://obot.example.com/mcp-connect-composite/vmcp1shared"
		wantTargetURL = "http://obot.obot-system.svc.cluster.local/mcp-connect-composite/vmcp1shared"
	)

	audienceURL, targetURL := compositeLoopbackURLs(serverURL, vmcpID, func(_ string) string {
		return internalBase + "/mcp-connect-composite/" + vmcpID
	})
	if audienceURL != wantPublicURL {
		t.Fatalf("vMCP composite audience URL = %q, want %q", audienceURL, wantPublicURL)
	}
	if targetURL != wantTargetURL {
		t.Fatalf("vMCP composite target URL = %q, want %q", targetURL, wantTargetURL)
	}
}
