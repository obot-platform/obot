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
upgradeNote: |
  ## Important

  Set the optional MODE value after updating.
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
	assert.Equal(t, "## Important\n\nSet the optional MODE value after updating.\n", entry.Spec.Manifest.UpgradeNote)
}

func TestReadGitCatalog(t *testing.T) {
	// This fixture repository uses the legacy schema. Valid Git URLs must
	// reach strict decoding and reject its entries, not fail at URL parsing.
	for _, test := range []struct {
		name         string
		catalog      string
		legacySchema bool
	}{
		{
			name:         "HTTPS legacy catalog",
			catalog:      "https://github.com/obot-platform/test-mcp-catalog",
			legacySchema: true,
		},
		{
			name:         "legacy catalog without protocol",
			catalog:      "github.com/obot-platform/test-mcp-catalog",
			legacySchema: true,
		},
		{
			name:         "legacy catalog with git suffix",
			catalog:      "https://github.com/obot-platform/test-mcp-catalog.git",
			legacySchema: true,
		},
		{
			name:    "invalid protocol",
			catalog: "http://github.com/obot-platform/test-mcp-catalog",
		},
		{
			name:    "invalid URL",
			catalog: "github.com/invalid",
		},
		{
			name:    "unknown host without git suffix",
			catalog: "https://self-hosted.example.com/org/repo",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries, err := readGitCatalogEntries[types.MCPServerCatalogEntryManifest](t.Context(), test.catalog, "")
			assert.Error(t, err)
			assert.Empty(t, entries)
			if test.legacySchema {
				assert.ErrorContains(t, err, "unknown field")
			}
		})
	}
}
