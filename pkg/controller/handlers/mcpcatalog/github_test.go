package mcpcatalog

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
)

func TestIsGitRepoURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/org/repo", true},
		{"https://gitlab.com/org/repo", true},
		{"https://example.com/org/repo.git", true},
		{"https://self-hosted.example.com/org/repo.git", true},
		{"https://example.com/some/raw/file.yaml", false},
		{"https://example.com/catalog.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, isGitRepoURL(tt.url))
		})
	}
}

func TestParseGitURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantClone    string
		wantBranch   string
		wantExplicit bool
		wantErr      bool
	}{
		{
			name:       "github without .git",
			url:        "https://github.com/org/repo",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "github with .git",
			url:        "https://github.com/org/repo.git",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "github with branch",
			url:        "https://github.com/org/repo/my-branch",
			wantClone:  "https://github.com/org/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:         "github with explicit ref",
			url:          "https://github.com/org/repo@other",
			wantClone:    "https://github.com/org/repo.git",
			wantBranch:   "other",
			wantExplicit: true,
		},
		{
			name:         "github without scheme with explicit ref",
			url:          "github.com/org/repo@other",
			wantClone:    "https://github.com/org/repo.git",
			wantBranch:   "other",
			wantExplicit: true,
		},
		{
			name:         "github .git with explicit ref",
			url:          "https://github.com/org/repo.git@tag",
			wantClone:    "https://github.com/org/repo.git",
			wantBranch:   "tag",
			wantExplicit: true,
		},
		{
			name:         "explicit ref overrides path branch",
			url:          "https://github.com/org/repo/my-branch@other",
			wantClone:    "https://github.com/org/repo.git",
			wantBranch:   "other",
			wantExplicit: true,
		},
		{
			name:       "gitlab with .git",
			url:        "https://gitlab.com/org/repo.git",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab subgroup",
			url:        "https://gitlab.com/group/subgroup/repo.git",
			wantClone:  "https://gitlab.com/group/subgroup/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab subgroup with branch",
			url:        "https://gitlab.com/group/subgroup/repo.git/my-branch",
			wantClone:  "https://gitlab.com/group/subgroup/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:       "gitlab without .git",
			url:        "https://gitlab.com/org/repo",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "main",
		},
		{
			name:       "gitlab with branch",
			url:        "https://gitlab.com/org/repo/my-branch",
			wantClone:  "https://gitlab.com/org/repo.git",
			wantBranch: "my-branch",
		},
		{
			name:    "bitbucket without .git",
			url:     "https://bitbucket.org/org/repo",
			wantErr: true,
		},
		{
			name:    "unknown host without .git is rejected",
			url:     "https://self-hosted.example.com/org/repo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloneURL, branch, explicitRef, err := parseGitURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantClone, cloneURL)
			assert.Equal(t, tt.wantBranch, branch)
			assert.Equal(t, tt.wantExplicit, explicitRef)
		})
	}
}

func TestGitCloneAuthAttempts(t *testing.T) {
	tests := []struct {
		name          string
		catalogToken  string
		fallbackToken string
		want          []gitCloneAuthAttempt
	}{
		{
			name:         "catalog token only",
			catalogToken: "catalog-token",
			want: []gitCloneAuthAttempt{
				{name: "catalog token", token: "catalog-token"},
			},
		},
		{
			name:          "catalog token ignores fallback token",
			catalogToken:  "catalog-token",
			fallbackToken: "fallback-token",
			want: []gitCloneAuthAttempt{
				{name: "catalog token", token: "catalog-token"},
			},
		},
		{
			name: "anonymous only",
			want: []gitCloneAuthAttempt{
				{name: "anonymous"},
			},
		},
		{
			name:          "anonymous then fallback token",
			fallbackToken: "fallback-token",
			want: []gitCloneAuthAttempt{
				{name: "anonymous"},
				{name: "fallback token", token: "fallback-token"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, gitCloneAuthAttempts(tt.catalogToken, tt.fallbackToken))
		})
	}
}

func TestReadGitCatalog(t *testing.T) {
	tests := []struct {
		name       string
		catalog    string
		wantErr    bool
		numEntries int
	}{
		{
			name:       "valid github url with https",
			catalog:    "https://github.com/obot-platform/test-mcp-catalog",
			wantErr:    false,
			numEntries: 3,
		},
		{
			name:       "valid github url without protocol",
			catalog:    "github.com/obot-platform/test-mcp-catalog",
			wantErr:    false,
			numEntries: 3,
		},
		{
			name:       "valid github url with .git suffix",
			catalog:    "https://github.com/obot-platform/test-mcp-catalog.git",
			wantErr:    false,
			numEntries: 3,
		},
		{
			name:       "invalid protocol",
			catalog:    "http://github.com/obot-platform/test-mcp-catalog",
			wantErr:    true,
			numEntries: 0,
		},
		{
			name:       "invalid url format",
			catalog:    "github.com/invalid",
			wantErr:    true,
			numEntries: 0,
		},
		{
			name:       "unknown host without .git suffix is rejected",
			catalog:    "https://self-hosted.example.com/org/repo",
			wantErr:    true,
			numEntries: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := readGitCatalogEntries[types.MCPServerCatalogEntryManifest](context.Background(), tt.catalog, "")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.numEntries, len(entries), "should return the correct number of catalog entries")

			// Verify that each entry has required fields
			for _, entry := range entries {
				// "Test 0" is in a file that should not have been included when reading the catalog.
				assert.NotEqual(t, entry.Name, "Test 0", "should not be the left out entry")

				assert.NotEmpty(t, entry.Name, "Name should not be empty")
				assert.NotEmpty(t, entry.Description, "Description should not be empty")
			}
		})
	}
}
