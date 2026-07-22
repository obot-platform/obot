package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	assert.NoError(t, err)
	assert.Len(t, objs, 1)

	entry, ok := objs[0].(*v1.MCPServerCatalogEntry)
	assert.True(t, ok)
	assert.Equal(t, dir, entry.Spec.SourceURL)
	assert.Equal(t, "test-entry", entry.Spec.Manifest.EntryKey)
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
			entries, err := readGitCatalogEntries[types.MCPServerCatalogEntryManifest](t.Context(), tt.catalog, "")
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
