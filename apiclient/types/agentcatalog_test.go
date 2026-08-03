package types

import (
	"strings"
	"testing"
)

// The manifest's own rules are the ones a client can apply too: shape, and an
// https repository. Anything about where this particular server will accept a
// repository from is not the manifest's business and is tested beside that
// policy instead.
func TestAgentCatalogManifestValidate(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		repoURL     string
		wantError   string
	}{
		{name: "https", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git"},
		{name: "missing displayName", repoURL: "https://example.com/obot/agents.git", wantError: "displayName is required"},
		{name: "missing repoURL", displayName: "Test agents", wantError: "repoURL is required"},
		{name: "absolute path", displayName: "Test agents", repoURL: "/home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "file URL", displayName: "Test agents", repoURL: "file:///home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "http", displayName: "Test agents", repoURL: "http://example.com/obot/agents.git", wantError: "must be an https URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AgentCatalogManifest{DisplayName: tt.displayName, RepoURL: tt.repoURL}.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}
