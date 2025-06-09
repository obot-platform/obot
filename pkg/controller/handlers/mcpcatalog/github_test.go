package mcpcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadGitHubCatalog(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := readGitHubCatalog(tt.catalog)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.numEntries, len(entries), "should return the correct number of catalog entries")

			// Verify that each entry has required fields
			for _, entry := range entries {
				// "Test 0" is in a file that should not have been included when reading the catalog.
				assert.NotEqual(t, entry.DisplayName, "Test 0", "should not be the left out entry")

				assert.NotEmpty(t, entry.ID, "ID should not be empty")
				assert.NotEmpty(t, entry.DisplayName, "DisplayName should not be empty")
				assert.NotEmpty(t, entry.Description, "Description should not be empty")
				assert.NotEmpty(t, entry.Manifest, "Manifest should not be empty")
			}
		})
	}
}
