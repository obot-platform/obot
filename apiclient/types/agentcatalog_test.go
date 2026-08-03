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
		ref         string
		wantError   string
	}{
		{name: "https", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git"},
		{name: "github owner and repo", displayName: "Test agents", repoURL: "https://github.com/obot-platform/agents"},
		{name: "ref", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git", ref: "release/v1.2"},
		{name: "empty ref", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git", ref: ""},
		{name: "missing displayName", repoURL: "https://example.com/obot/agents.git", wantError: "displayName is required"},
		{name: "missing repoURL", displayName: "Test agents", wantError: "repoURL is required"},
		{name: "absolute path", displayName: "Test agents", repoURL: "/home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "file URL", displayName: "Test agents", repoURL: "file:///home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "http", displayName: "Test agents", repoURL: "http://example.com/obot/agents.git", wantError: "must be an https URL"},
		{name: "embedded credentials", displayName: "Test agents", repoURL: "https://user:token@example.com/obot/agents.git", wantError: "must not include credentials"},
		{name: "no git boundary", displayName: "Test agents", repoURL: "https://example.com/obot/agents", wantError: ".git repository boundary"},
		{name: "github without repo", displayName: "Test agents", repoURL: "https://github.com/obot-platform", wantError: "must include an owner and repository"},
		{name: "ref beginning with dash", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git", ref: "--upload-pack=evil", wantError: "must not begin with '-'"},
		{name: "whitespace-only ref", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git", ref: "   ", wantError: "may only contain"},
		{name: "ref with revision range", displayName: "Test agents", repoURL: "https://example.com/obot/agents.git", ref: "main..evil", wantError: "must not contain '..'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AgentCatalogManifest{DisplayName: tt.displayName, RepoURL: tt.repoURL, Ref: tt.ref}.Validate()
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
