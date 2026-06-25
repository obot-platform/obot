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
	assert.Equal(t, dir, sourceID)
	assert.Len(t, objs, 1)
}

func TestReadMCPCatalogRejectsSeparatorInIDRef(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`idRef: bad|ref
name: Bad
shortDescription: Bad
description: Bad
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, sourceID, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `source entry ref "bad|ref" cannot contain |`)
	assert.Equal(t, dir, sourceID)
	assert.Empty(t, objs)
}

func TestSourceIDForURL(t *testing.T) {
	assert.Equal(t, "github.com/company/mcp-catalog", sourceIDForURL("https://github.com/company/mcp-catalog"))
	assert.Equal(t, "github.com/company/mcp-catalog", sourceIDForURL("github.com/company/mcp-catalog"))
	assert.Equal(t, "/tmp/catalog", sourceIDForURL("/tmp/catalog"))
}
