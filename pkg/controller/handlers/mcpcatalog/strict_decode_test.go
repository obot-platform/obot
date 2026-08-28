package mcpcatalog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestReadCatalogManifestsStrict(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "valid",
			content: "- name: Test\n",
		},
		{
			name:    "unknown field",
			content: "- name: Test\n  unknownField: true\n",
			wantErr: true,
		},
		{
			name:    "removed env field",
			content: "- name: Test\n  env: []\n",
			wantErr: true,
		},
		{
			name:    "removed headers field",
			content: "- name: Test\n  remoteConfig:\n    headers: []\n",
			wantErr: true,
		},
		{
			name:    "removed server user type",
			content: "- name: Test\n  serverUserType: singleUser\n",
			wantErr: true,
		},
		{
			name:    "duplicate field",
			content: "- name: Test\n  name: Duplicate\n",
			wantErr: true,
		},
		{
			name:    "mixed list is skipped as a whole",
			content: "- name: Valid\n- name: Invalid\n  unknownField: true\n",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "catalog.yaml")
			require.NoError(t, os.WriteFile(file, []byte(test.content), 0o600))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, test.content)
			}))
			defer server.Close()
			for _, source := range []string{file, server.URL} {
				entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), server.Client(), source, "")
				if test.wantErr {
					require.Error(t, err)
					require.Empty(t, entries)
				} else {
					require.NoError(t, err)
					require.Len(t, entries, 1)
				}
			}
		})
	}
}

func TestReadMCPCatalogRetainsPartialResultsAndReportsIncompleteSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "valid.yaml"), []byte(`name: Valid
entryKey: valid
shortDescription: Test
description: Test
icon: icon
runtime: npx
npxConfig:
  package: test
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "legacy.yaml"), []byte("name: Legacy\nenv: []\n"), 0o600))

	// The error must survive every reader layer. SyncErrors then prevents
	// reconciliation from deleting or detaching the skipped entry.
	entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), http.DefaultClient, dir, "")
	require.ErrorContains(t, err, "legacy.yaml")
	require.Len(t, entries, 1)
	require.Equal(t, "Valid", entries[0].Name)

	h := &Handler{}
	objects, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	require.ErrorContains(t, err, "legacy.yaml")
	require.Len(t, objects, 1)
}
