package mcpcatalog

import (
	"context"
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
	objs, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `duplicate source entry key "shared"`)
	assert.Len(t, objs, 1)
}

func TestReadMCPCatalogRejectsSeparatorInEntryKey(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`entryKey: bad::key
name: Bad
shortDescription: Bad
description: Bad
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))

	h := &Handler{}
	objs, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `source entry key "bad::key" cannot contain ::`)
	assert.Empty(t, objs)
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
	objs, err := h.readMCPCatalog(context.Background(), "default", dir, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), `source entry key "Bad_Key" must be DNS-friendly`)
	assert.Empty(t, objs)
}

func TestParseSourceRef(t *testing.T) {
	const currentSourceID = "current-source"

	tests := []struct {
		name           string
		catalogEntryID string
		wantSourceID   string
		wantEntryKey   string
		wantHasSep     bool
		wantValid      bool
	}{
		{
			name:           "same source shorthand",
			catalogEntryID: "gmail",
			wantSourceID:   currentSourceID,
			wantEntryKey:   "gmail",
			wantHasSep:     false,
			wantValid:      true,
		},
		{
			name:           "explicit source",
			catalogEntryID: "other-source::gmail",
			wantSourceID:   "other-source",
			wantEntryKey:   "gmail",
			wantHasSep:     true,
			wantValid:      true,
		},
		{
			name:           "missing source",
			catalogEntryID: "::gmail",
			wantEntryKey:   "gmail",
			wantHasSep:     true,
			wantValid:      false,
		},
		{
			name:           "missing entry key",
			catalogEntryID: "source::",
			wantSourceID:   "source",
			wantHasSep:     true,
			wantValid:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceID, entryKey, hasSep, valid := parseSourceRef(currentSourceID, tt.catalogEntryID)

			assert.Equal(t, tt.wantSourceID, sourceID)
			assert.Equal(t, tt.wantEntryKey, entryKey)
			assert.Equal(t, tt.wantHasSep, hasSep)
			assert.Equal(t, tt.wantValid, valid)
		})
	}
}
