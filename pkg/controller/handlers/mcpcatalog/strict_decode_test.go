package mcpcatalog

import (
	"bytes"
	"fmt"
	"log/slog"
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
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit selection=%t", explicit), func(t *testing.T) {
			dir := t.TempDir()
			if explicit {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".obotcatalogs"), []byte("*.yaml\n"), 0o600))
			}

			writeParseTestManifest(t, dir, "Valid", "original")
			require.NoError(t, os.WriteFile(filepath.Join(dir, "legacy.yaml"), []byte("name: Legacy\nenv: []\n"), 0o600))
			corruptParseTestManifest(t, writeParseTestManifest(t, dir, "Broken", "original"))

			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			// Both schema and syntax errors must survive every reader layer
			// alongside valid entries, so sync can identify an incomplete source.
			entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), http.DefaultClient, dir, "")
			require.ErrorContains(t, err, "legacy.yaml")
			require.ErrorContains(t, err, "Broken.yaml")
			require.Len(t, entries, 1)
			require.Equal(t, "Valid", entries[0].Name)

			require.Contains(t, logs.String(), "level=WARN")
			require.Contains(t, logs.String(), "legacy.yaml")
			require.Contains(t, logs.String(), "Broken.yaml")

			objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")
			require.ErrorContains(t, err, "legacy.yaml")
			require.ErrorContains(t, err, "Broken.yaml")
			require.Len(t, objects, 1)
		})
	}
}
