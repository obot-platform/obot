package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
)

func TestReadMCPCatalogSetsSourceMetadata(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`entryKey: test-entry
name: Test
shortDescription: Test
description: Test
icon: icon
upgradeNote: |
  ## Important

  Set the optional MODE value after updating.
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, _, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	assert.NoError(t, err)
	assert.Len(t, objs, 1)

	entry, ok := objs[0].(*v1.MCPServerCatalogEntry)
	assert.True(t, ok)
	assert.Equal(t, dir, entry.Spec.SourceURL)
	assert.Equal(t, "test-entry", entry.Spec.Manifest.EntryKey)
	assert.Equal(t, "## Important\n\nSet the optional MODE value after updating.\n", entry.Spec.Manifest.UpgradeNote)
}

func TestReadGitCatalogEntries(t *testing.T) {
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
			entries, _, err := readGitCatalogEntries[types.MCPServerCatalogEntryManifest](t.Context(), tt.catalog, "")
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

func TestNextResolvedCommitSHAsOnlyKeepsConfiguredGitSources(t *testing.T) {
	current := map[string]string{
		"https://github.com/example/unchanged": "old-sha",
		"https://example.com/catalog.yaml":     "",
		"https://github.com/example/removed":   "removed-sha",
	}
	successful := map[string]string{
		"https://github.com/example/changed": "new-sha",
	}

	next := nextResolvedCommitSHAs(current, successful, []string{
		"https://github.com/example/unchanged",
		"https://github.com/example/changed",
		"https://example.com/catalog.yaml",
	})

	assert.Equal(t, map[string]string{
		"https://github.com/example/unchanged": "old-sha",
		"https://github.com/example/changed":   "new-sha",
	}, next)
}

func TestHasChangedPreviouslySyncedSource(t *testing.T) {
	tests := []struct {
		name       string
		previous   map[string]string
		successful map[string]string
		want       bool
	}{
		{
			name:       "existing source changed",
			previous:   map[string]string{"source-a": "old"},
			successful: map[string]string{"source-a": "new"},
			want:       true,
		},
		{
			name:       "new source does not force existing sources",
			previous:   map[string]string{"source-a": "same"},
			successful: map[string]string{"source-b": "new"},
		},
		{
			name:       "unchanged source",
			previous:   map[string]string{"source-a": "same"},
			successful: map[string]string{"source-a": "same"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, hasChangedPreviouslySyncedSource(test.previous, test.successful))
		})
	}
}

func TestPreviouslyAppliedUnversionedSourceRequiresCompleteReconciliation(t *testing.T) {
	sourceURLs := []string{"https://example.com/catalog.yaml", "https://github.com/example/catalog"}
	previousCommits := map[string]string{"https://github.com/example/catalog": "sha"}
	validSourceIDs := map[string]struct{}{mcp.SourceIDForURL(sourceURLs[0]): {}}
	unversionedSourceIDs := validUnversionedSourceIDs(sourceURLs, previousCommits, validSourceIDs)

	assert.True(t, containsAnySourceID(map[string]struct{}{
		mcp.SourceIDForURL(sourceURLs[0]): {},
	}, unversionedSourceIDs))
}

func TestNewUnversionedSourceDoesNotRequireCompleteReconciliation(t *testing.T) {
	sourceURLs := []string{"https://example.com/new-catalog.yaml", "https://github.com/example/catalog"}
	previousCommits := map[string]string{"https://github.com/example/catalog": "sha"}
	validSourceIDs := map[string]struct{}{mcp.SourceIDForURL(sourceURLs[0]): {}}
	unversionedSourceIDs := validUnversionedSourceIDs(sourceURLs, previousCommits, validSourceIDs)

	assert.NotEmpty(t, unversionedSourceIDs)
	assert.False(t, containsAnySourceID(nil, unversionedSourceIDs))
}
