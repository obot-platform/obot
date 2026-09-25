package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

const (
	generateTestSourceURL = "https://github.com/example/catalog"
)

func generateTestItems() []types.OrphanedVMCPCatalogItem {
	return []types.OrphanedVMCPCatalogItem{
		{
			VMCPID:      "vmcp1",
			DisplayName: "Email",
			EntryKey:    "email",
			YAML:        "type: vmcp\nentryKey: email\ndisplayName: Email\ncomponents:\n  - id: gmail\n    name: Gmail\n    mcpServerCatalogEntryKey: obot-gmail\nprofiles: []\n",
		},
		{
			VMCPID:      "vmcp2",
			DisplayName: "Research/Docs",
			EntryKey:    "research",
			YAML:        "type: vmcp\nentryKey: research\ndisplayName: Research/Docs\ncomponents: []\nprofiles: []\n",
		},
	}
}

func generateTestServer(t *testing.T, items []types.OrphanedVMCPCatalogItem) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/api-keys/auth":
			_, _ = w.Write([]byte(`{"allowed": true, "scopes": {"canAccessAPI": true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/mcp-catalogs/default/orphaned-vmcps":
			if got := r.URL.Query().Get("sourceURL"); got != generateTestSourceURL {
				t.Errorf("sourceURL = %q, want %q", got, generateTestSourceURL)
			}
			_ = json.NewEncoder(w).Encode(types.OrphanedVMCPCatalogItemList{Items: items})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func executeGenerateTestCommand(t *testing.T, server *httptest.Server, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := cmd.Command(&MCP{root: mcpTestRoot(server.URL)})
	command.SetContext(t.Context())
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs(append([]string{"generate-vmcp-catalog-yaml"}, args...))
	err := command.Execute()
	return stdout.String(), stderr.String(), err
}

func TestMCPGenerateVMCPCatalogYAMLStdoutWritesOneCatalogList(t *testing.T) {
	stdout, _, err := executeGenerateTestCommand(t, generateTestServer(t, generateTestItems()), generateTestSourceURL)
	require.NoError(t, err)
	require.Equal(t, `- type: vmcp
  entryKey: email
  displayName: Email
  components:
    - id: gmail
      name: Gmail
      mcpServerCatalogEntryKey: obot-gmail
  profiles: []
- type: vmcp
  entryKey: research
  displayName: Research/Docs
  components: []
  profiles: []
`, stdout)

	// Catalog sync reads a file as one YAML document, so the output must be a list it can validate.
	path := filepath.Join(t.TempDir(), "vmcps.yaml")
	require.NoError(t, os.WriteFile(path, []byte(stdout), 0o644))
	require.NoError(t, validateMCPCatalogFile(t.Context(), path, true, map[string]string{}, map[string]string{}))
}

func TestMCPGenerateVMCPCatalogYAMLFileRefusesToOverwrite(t *testing.T) {
	server := generateTestServer(t, generateTestItems())
	path := filepath.Join(t.TempDir(), "vmcps.yaml")
	require.NoError(t, os.WriteFile(path, []byte("existing"), 0o644))

	_, _, err := executeGenerateTestCommand(t, server, generateTestSourceURL, "--mode", "file", path)
	require.ErrorContains(t, err, "already exists; use --overwrite")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "existing", string(data))

	_, _, err = executeGenerateTestCommand(t, server, generateTestSourceURL, "--mode", "file", "--overwrite", path)
	require.NoError(t, err)
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(data), "- type: vmcp\n"), string(data))
}

func TestMCPGenerateVMCPCatalogYAMLDirWritesFilePerVMCP(t *testing.T) {
	items := generateTestItems()
	// A second vMCP with the same display name must not replace the first.
	items = append(items, types.OrphanedVMCPCatalogItem{
		VMCPID:      "vmcp3",
		DisplayName: "email",
		EntryKey:    "email-2",
		YAML:        "type: vmcp\nentryKey: email-2\ndisplayName: email\ncomponents: []\nprofiles: []\n",
	})
	server := generateTestServer(t, items)
	dir := filepath.Join(t.TempDir(), "vmcps")

	_, _, err := executeGenerateTestCommand(t, server, generateTestSourceURL, "--mode", "dir", dir)
	require.NoError(t, err)
	for name, item := range map[string]types.OrphanedVMCPCatalogItem{
		"Email.yaml":         items[0],
		"Research-Docs.yaml": items[1],
		"email-email-2.yaml": items[2],
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, item.YAML, string(data))
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Email.yaml"), []byte("edited"), 0o644))
	_, stderr, err := executeGenerateTestCommand(t, server, generateTestSourceURL, "--mode", "dir", dir)
	require.ErrorContains(t, err, "3 orphaned vMCPs were not generated")
	require.Contains(t, stderr, "Email.yaml already exists; use --overwrite")
	data, err := os.ReadFile(filepath.Join(dir, "Email.yaml"))
	require.NoError(t, err)
	require.Equal(t, "edited", string(data))

	_, _, err = executeGenerateTestCommand(t, server, generateTestSourceURL, "--mode", "dir", "--overwrite", dir)
	require.NoError(t, err)
	data, err = os.ReadFile(filepath.Join(dir, "Email.yaml"))
	require.NoError(t, err)
	require.Equal(t, items[0].YAML, string(data))
}

func TestMCPGenerateVMCPCatalogYAMLReportsSkippedVMCPs(t *testing.T) {
	items := append(generateTestItems()[:1], types.OrphanedVMCPCatalogItem{
		VMCPID:      "vmcp2",
		DisplayName: "Broken",
		Error:       "component has no entryKey",
	})

	stdout, stderr, err := executeGenerateTestCommand(t, generateTestServer(t, items), generateTestSourceURL)
	require.ErrorContains(t, err, "1 orphaned vMCPs were not generated")
	require.Contains(t, stderr, `Skipping vMCP "Broken": component has no entryKey`)
	require.Contains(t, stdout, "entryKey: email")
	require.NotContains(t, stdout, "Broken")
}

func TestMCPGenerateVMCPCatalogYAMLNothingToGenerate(t *testing.T) {
	stdout, stderr, err := executeGenerateTestCommand(t, generateTestServer(t, nil), generateTestSourceURL, "--mode", "file", filepath.Join(t.TempDir(), "vmcps.yaml"))
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Contains(t, stderr, "No orphaned vMCPs from https://github.com/example/catalog are awaiting catalog sync.")
}

func TestMCPGenerateVMCPCatalogYAMLValidatesArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing URL",
			args: []string{},
			want: "accepts between 1 and 2 arg(s)",
		},
		{
			name: "stdout with path",
			args: []string{generateTestSourceURL, "out.yaml"},
			want: "--mode stdout does not take a path",
		},
		{
			name: "file without path",
			args: []string{generateTestSourceURL, "--mode", "file"},
			want: "--mode file requires a path",
		},
		{
			name: "dir without path",
			args: []string{generateTestSourceURL, "--mode", "dir"},
			want: "--mode dir requires a path",
		},
		{
			name: "unknown mode",
			args: []string{generateTestSourceURL, "--mode", "zip"},
			want: `invalid --mode "zip"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			}))
			t.Cleanup(server.Close)

			_, _, err := executeGenerateTestCommand(t, server, tc.args...)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestVMCPFileName(t *testing.T) {
	for _, tc := range []struct {
		displayName string
		want        string
	}{
		{
			displayName: "Email",
			want:        "Email.yaml",
		},
		{
			displayName: "Research / Docs: v2?",
			want:        "Research - Docs- v2-.yaml",
		},
		{
			displayName: "../secret",
			want:        "-secret.yaml",
		},
		{
			displayName: " .. ",
			want:        "vmcp.yaml",
		},
	} {
		t.Run(tc.displayName, func(t *testing.T) {
			require.Equal(t, tc.want, vmcpFileName(tc.displayName))
		})
	}
}
