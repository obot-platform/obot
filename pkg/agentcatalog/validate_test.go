package agentcatalog

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// Accepting a repository off the local filesystem is a development-mode
// concession, so the same manifest has to be judged differently depending on
// how this server is running -- which is the whole reason the rule lives here
// rather than on the manifest.
func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		repoURL     string
		ref         string
		development bool
		wantError   string
	}{
		{name: "https production", repoURL: "https://example.com/obot/agents.git"},
		{name: "https development", repoURL: "https://example.com/obot/agents.git", development: true},
		{name: "absolute path development", repoURL: "/home/developer/src/obot-agents", development: true},
		{name: "file URL development", repoURL: "file:///home/developer/src/obot-agents", development: true},
		{name: "localhost file URL development", repoURL: "file://localhost/home/developer/src/obot-agents", development: true},
		{name: "absolute path production", repoURL: "/home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "file URL production", repoURL: "file:///home/developer/src/obot-agents", wantError: "must be an https URL"},
		{name: "relative path development", repoURL: "../obot-agents", development: true, wantError: "must be an https URL"},
		{name: "remote file host development", repoURL: "file://example.com/repo", development: true, wantError: "host must be empty or localhost"},
		{name: "file query development", repoURL: "file:///repo?ref=main", development: true, wantError: "must not include"},
		{name: "displayName still required in development", repoURL: "/home/developer/src/obot-agents", development: true, wantError: "displayName is required"},
		{name: "ref still validated in development", repoURL: "/home/developer/src/obot-agents", development: true, ref: "--upload-pack=evil", wantError: "must not begin with '-'"},
		{name: "valid ref in development", repoURL: "/home/developer/src/obot-agents", development: true, ref: "main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := types.AgentCatalogManifest{DisplayName: "Test agents", RepoURL: tt.repoURL, Ref: tt.ref}
			if strings.Contains(tt.name, "displayName still required") {
				manifest.DisplayName = ""
			}
			err := Validate(manifest, tt.development)
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
