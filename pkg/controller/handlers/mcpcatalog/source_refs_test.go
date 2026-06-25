package mcpcatalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
)

func TestReadMCPCatalogSetsFileRefs(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o700))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "first.yaml"), []byte(`name: First
shortDescription: First
description: First
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, sourceID, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.NoError(t, err)
	assert.Equal(t, dir, sourceID)
	assert.Len(t, objs, 1)
	assert.Equal(t, "nested/first.yaml", objs[0].(*v1.MCPServerCatalogEntry).Spec.SourceEntryFileRef)
}

func TestReadMCPCatalogDoesNotSetFileRefForMultiEntryFile(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entries.yaml"), []byte(`- name: First
  shortDescription: First
  description: First
  icon: icon
  runtime: npx
  npxConfig:
    package: test
- name: Second
  shortDescription: Second
  description: Second
  icon: icon
  runtime: npx
  npxConfig:
    package: test
`), 0o600))

	h := &Handler{}
	objs, sourceID, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.NoError(t, err)
	assert.Equal(t, dir, sourceID)
	assert.Len(t, objs, 2)
	assert.Empty(t, objs[0].(*v1.MCPServerCatalogEntry).Spec.SourceEntryFileRef)
	assert.Empty(t, objs[1].(*v1.MCPServerCatalogEntry).Spec.SourceEntryFileRef)
}

func TestSourceIDForURL(t *testing.T) {
	assert.Equal(t, "github.com/company/mcp-catalog", sourceIDForURL("https://github.com/company/mcp-catalog"))
	assert.Equal(t, "github.com/company/mcp-catalog", sourceIDForURL("github.com/company/mcp-catalog"))
	assert.Equal(t, "/tmp/catalog", sourceIDForURL("/tmp/catalog"))
}
