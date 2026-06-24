package mcpcatalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadMCPCatalogReturnsDuplicateIDRefErrors(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, obotCatalogMetadataFilename+".yaml"), []byte("id: acme/catalog\n"), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "first.yaml"), []byte(`idRef: shared
name: First
shortDescription: First
description: First
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "second.yaml"), []byte(`idRef: shared
name: Second
shortDescription: Second
description: Second
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, sourceID, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate source entry ref "shared"`)
	assert.Equal(t, "acme/catalog", sourceID)
	assert.Len(t, objs, 1)
}
