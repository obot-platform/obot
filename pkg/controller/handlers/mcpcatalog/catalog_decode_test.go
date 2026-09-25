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

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReadCatalogManifestsDecoding(t *testing.T) {
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
		},
		{
			name:    "removed env field",
			content: "- name: Test\n  env: []\n",
		},
		{
			name:    "removed headers field",
			content: "- name: Test\n  remoteConfig:\n    headers: []\n",
		},
		{
			name:    "removed server user type",
			content: "- name: Test\n  serverUserType: singleUser\n",
		},
		{
			name:    "duplicate field",
			content: "- name: Test\n  name: Duplicate\n",
			wantErr: true,
		},
		{
			name:    "mixed list keeps both entries",
			content: "- name: Valid\n- name: Invalid\n  unknownField: true\n",
		},
		{
			name:    "invalid syntax",
			content: "- name: Test\n  unknownField: [\n",
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
				entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), server.Client(), source, "", 100)
				if test.wantErr {
					require.Error(t, err)
					require.Empty(t, entries)
				} else {
					require.NoError(t, err)
					if test.name == "mixed list keeps both entries" {
						require.Len(t, entries, 2)
					} else {
						require.Len(t, entries, 1)
					}
				}
			}
		})
	}
}

func TestReadMCPCatalogMixedJSON(t *testing.T) {
	content := `[
  {
    "type": "",
    "entryKey": "search",
    "name": "Search",
    "shortDescription": "Search",
    "description": "Search",
    "icon": "icon",
    "runtime": "npx",
    "npxConfig": {"package": "search"}
  },
  {
    "type": "vmcp",
    "entryKey": "bundle",
    "displayName": "Bundle",
    "components": [{"id": "search", "name": "Search", "mcpServerCatalogEntryKey": "search"}]
  }
]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, content)
	}))
	defer server.Close()

	objects, err := (&Handler{httpClient: server.Client()}).readMCPCatalog(t.Context(), "default", server.URL, "")
	require.NoError(t, err)
	require.Len(t, objects, 2)
	require.IsType(t, &v1.MCPServerCatalogEntry{}, objects[0])
	require.IsType(t, &v1.VMCP{}, objects[1])

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{server.URL}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)
	handler.httpClient = server.Client()
	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{}))
	require.Empty(t, catalog.Status.SyncErrors)

	var vmcps v1.VMCPList
	require.NoError(t, client.List(t.Context(), &vmcps, kclient.InNamespace(catalog.Namespace)))
	require.Len(t, vmcps.Items, 1)
}

func TestReadMCPCatalogCoercesNumericArgs(t *testing.T) {
	content := `- entryKey: example
  name: Example
  shortDescription: Example
  description: Example
  icon: icon
  runtime: containerized
  containerizedConfig:
    image: example/mcp:latest
    port: 8080
    path: /mcp
    args: [one, two, 123]
`
	file := filepath.Join(t.TempDir(), "catalog.yaml")
	require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, content)
	}))
	defer server.Close()

	for _, source := range []string{file, server.URL} {
		objects, err := (&Handler{httpClient: server.Client()}).readMCPCatalog(t.Context(), "default", source, "")
		require.NoError(t, err)
		require.Len(t, objects, 1)
		entry := objects[0].(*v1.MCPServerCatalogEntry)
		require.Equal(t, []string{"one", "two", "123"}, entry.Spec.Manifest.ContainerizedConfig.Args)
	}
}

func TestReadMCPCatalogRejectsUnknownType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.yaml")
	require.NoError(t, os.WriteFile(path, []byte("- type: unknown\n  name: Test\n"), 0o600))

	objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", path, "")
	require.ErrorContains(t, err, `unsupported catalog item type "unknown"`)
	require.Empty(t, objects)
}

func TestCatalogSyncRejectsLegacyConfiguration(t *testing.T) {
	for _, field := range []string{
		"env: []",
		"remoteConfig: {headers: []}",
		"multiUserConfig: {userDefinedHeaders: null}",
	} {
		t.Run(field, func(t *testing.T) {
			content := "- name: Legacy\n  " + field + "\n"
			path := filepath.Join(t.TempDir(), "catalog.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, content)
			}))
			defer server.Close()

			handler := &Handler{httpClient: server.Client()}
			for _, source := range []string{path, server.URL} {
				objects, err := handler.readMCPCatalog(t.Context(), "default", source, "")
				require.ErrorContains(t, err, "top-level config")
				require.Empty(t, objects)

				objects, err = handler.readSystemMCPCatalog(t.Context(), "default", source, "")
				require.ErrorContains(t, err, "top-level config")
				require.Empty(t, objects)
			}
		})
	}
}

func TestReadMCPCatalogRequiresVMCPEntryKeyReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.yaml")
	content := `- type: vmcp
  displayName: Email
  components:
    - id: gmail
      mcpServerCatalogEntryID: obot-gmail
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", path, "")
	require.ErrorContains(t, err, `vMCP "Email" component "gmail" mcpServerCatalogEntryKey is required`)
	require.Empty(t, objects)
}

func TestReadMCPCatalogRequiresVMCPComponentID(t *testing.T) {
	for _, id := range []string{"", "  "} {
		t.Run(fmt.Sprintf("id=%q", id), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "catalog.yaml")
			content := fmt.Sprintf(`- type: vmcp
  displayName: Email
  components:
    - id: %q
      name: Gmail
      mcpServerCatalogEntryKey: obot-gmail
`, id)
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

			objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", path, "")
			require.ErrorContains(t, err, `vMCP "Email" components[0] id is required`)
			require.Empty(t, objects)
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
			legacy := writeParseTestManifest(t, dir, "Legacy", "original")
			contents, err := os.ReadFile(legacy)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(legacy, append(contents, []byte("env: []\n")...), 0o600))
			corruptParseTestManifest(t, writeParseTestManifest(t, dir, "Broken", "original"))

			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
			t.Cleanup(func() { slog.SetDefault(previous) })

			// Unknown fields stay compatible, while syntax errors still mark
			// the source incomplete without hiding valid entries.
			entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), http.DefaultClient, dir, "", 100)
			require.ErrorContains(t, err, "Broken.yaml")
			require.Len(t, entries, 2)

			require.Contains(t, logs.String(), "level=WARN")
			require.Contains(t, logs.String(), "Broken.yaml")

			objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")
			require.ErrorContains(t, err, "Broken.yaml")
			require.ErrorContains(t, err, `legacy configuration field "env"`)
			require.Len(t, objects, 1)
		})
	}
}

func TestReadMCPCatalogSkipsGitHubWorkflows(t *testing.T) {
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

	workflowDir := filepath.Join(dir, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte("name: Validate catalog\non: push\njobs: {}\n"), 0o600))

	h := &Handler{}
	objects, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	require.Len(t, objects, 1)
}
