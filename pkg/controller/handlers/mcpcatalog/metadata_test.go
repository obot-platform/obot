package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadGitCatalogMetadata(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantID   string
		wantErr  bool
		errMatch string
	}{
		{
			name:   "missing metadata",
			wantID: "",
		},
		{
			name: "yaml metadata",
			files: map[string]string{
				obotCatalogMetadataFilename + ".yaml": "id: https://github.com/acme/mcp-catalog\n",
			},
			wantID: "https://github.com/acme/mcp-catalog",
		},
		{
			name: "yml metadata",
			files: map[string]string{
				obotCatalogMetadataFilename + ".yml": "id: acme/catalog\n",
			},
			wantID: "acme/catalog",
		},
		{
			name: "empty id",
			files: map[string]string{
				obotCatalogMetadataFilename + ".yaml": "id: \n",
			},
			wantErr:  true,
			errMatch: ".obotcatalog.yaml id is required",
		},
		{
			name: "separator in id",
			files: map[string]string{
				obotCatalogMetadataFilename + ".yaml": "id: acme" + catalogReferenceSeparator + "catalog\n",
			},
			wantErr:  true,
			errMatch: ".obotcatalog.yaml id cannot contain " + catalogReferenceSeparator,
		},
		{
			name: "yaml metadata preferred over yml",
			files: map[string]string{
				obotCatalogMetadataFilename + ".yaml": "id: acme/catalog\n",
				obotCatalogMetadataFilename + ".yml":  "id: other/catalog\n",
			},
			wantID: "acme/catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for filename, contents := range tt.files {
				err := os.WriteFile(filepath.Join(dir, filename), []byte(contents), 0o600)
				assert.NoError(t, err)
			}

			id, err := readGitCatalogMetadata(dir)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMatch)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantID, id)
		})
	}
}
