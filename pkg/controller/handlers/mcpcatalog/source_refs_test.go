package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadMCPCatalogReturnsDuplicateEntryKeyErrors(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "first.yaml"), []byte(`entryKey: shared
name: First
shortDescription: First
description: First
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "second.yaml"), []byte(`entryKey: shared
name: Second
shortDescription: Second
description: Second
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, _, err := h.readMCPCatalog(t.Context(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate source entry key "shared"`)
	assert.Len(t, objs, 1)
}

func TestReadMCPCatalogRejectsNonDNSFriendlyEntryKey(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`entryKey: Bad_Key
name: Bad
shortDescription: Bad
description: Bad
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, _, err := h.readMCPCatalog(t.Context(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `source entry key "Bad_Key" must be DNS-friendly`)
	assert.Empty(t, objs)
}

func TestReadMCPCatalogRejectsTunnelName(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`name: Private MCP
shortDescription: Private MCP
description: Private MCP
runtime: remote
remoteConfig:
  fixedURL: https://mcp.internal.example/mcp
  tunnelName: mt1office
`), 0o600))

	h := &Handler{}
	objs, _, err := h.readMCPCatalog(t.Context(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remoteConfig.tunnelName: cannot be set on catalog-synced entries")
	assert.Empty(t, objs)
}
