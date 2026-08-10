package mcpgateway

import "testing"

func TestCompositeLoopbackURLsUsesBackendTargetAndPublicAudience(t *testing.T) {
	const (
		serverURL       = "https://obot.example.com/"
		mcpServerName   = "mcp-composite"
		internalBaseURL = "http://obot.obot-system.svc.cluster.local"
	)

	var transformedURL string
	audienceURL, targetURL := compositeLoopbackURLs(serverURL, mcpServerName, func(rawURL string) string {
		transformedURL = rawURL
		return internalBaseURL + "/mcp-connect-composite/" + mcpServerName
	})

	wantAudienceURL := "https://obot.example.com/mcp-connect-composite/mcp-composite"
	if audienceURL != wantAudienceURL {
		t.Fatalf("audience URL = %q, want %q", audienceURL, wantAudienceURL)
	}
	if transformedURL != wantAudienceURL {
		t.Fatalf("URL passed to backend transform = %q, want %q", transformedURL, wantAudienceURL)
	}
	wantTargetURL := "http://obot.obot-system.svc.cluster.local/mcp-connect-composite/mcp-composite"
	if targetURL != wantTargetURL {
		t.Fatalf("target URL = %q, want %q", targetURL, wantTargetURL)
	}
}
