package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
)

func TestMCPSearchPaginatesAndWritesTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/v0.1/servers" {
			t.Fatalf("path = %s, want /v0.1/servers", r.URL.Path)
		}
		if got := r.URL.Query().Get("search"); got != "github issues" {
			t.Fatalf("search = %q, want github issues", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}

		switch r.URL.Query().Get("cursor") {
		case "":
			if got := r.URL.Query().Get("limit"); got != "3" {
				t.Fatalf("first page limit = %q, want 3", got)
			}
			_ = json.NewEncoder(w).Encode(types.RegistryServerList{
				Servers: []types.RegistryServerResponse{
					registryTestServer("io.example.one", "One", "first", "https://obot.example.com/mcp-connect/one", false),
					registryTestServer("io.example/two", "Two", "second", "", true),
				},
				Metadata: &types.RegistryServerListMetadata{NextCursor: "two", Count: 2},
			})
		case "two":
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Fatalf("second page limit = %q, want 1", got)
			}
			_ = json.NewEncoder(w).Encode(types.RegistryServerList{
				Servers: []types.RegistryServerResponse{
					registryTestServer("io.example.three", "Three", "third", "", false),
				},
				Metadata: &types.RegistryServerListMetadata{Count: 1},
			})
		default:
			t.Fatalf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	defer server.Close()

	stdout, err := executeMCPTestCommand(t, mcpTestRoot(server.URL), "search", "github", "issues", "--limit", "3")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"TITLE", "DESCRIPTION", "STATUS", "URL",
		"One", "first", "ready", "https://obot.example.com/mcp-connect/one",
		"Two", "second", "configuration required", server.URL + "/mcp-servers/c/two",
		"Three", "third", "unknown",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "io.example.one") {
		t.Fatalf("human table should not include registry name:\n%s", stdout)
	}
}

func TestMCPSearchJSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0.1/servers" {
			t.Fatalf("path = %s, want /v0.1/servers", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(types.RegistryServerList{
			Servers: []types.RegistryServerResponse{
				registryTestServer("io.example.github", "GitHub", "GitHub MCP server", "https://obot.example.com/mcp-connect/github", false),
			},
			Metadata: &types.RegistryServerListMetadata{Count: 1},
		})
	}))
	defer server.Close()

	stdout, err := executeMCPTestCommand(t, mcpTestRoot(server.URL), "search", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var result mcpSearchOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if len(result.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(result.Servers))
	}
	got := result.Servers[0]
	if got.Name != "io.example.github" || got.Status != "ready" || got.URL == "" || got.ConfigurationRequired {
		t.Fatalf("unexpected JSON result: %#v", got)
	}
}

func TestMCPSearchJSONModeIncludesConfigurationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0.1/servers" {
			t.Fatalf("path = %s, want /v0.1/servers", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(types.RegistryServerList{
			Servers: []types.RegistryServerResponse{
				registryTestServer("io.example/ms1server", "Personal", "needs setup", "", true),
			},
			Metadata: &types.RegistryServerListMetadata{Count: 1},
		})
	}))
	defer server.Close()

	stdout, err := executeMCPTestCommand(t, mcpTestRoot(server.URL), "search", "--json")
	if err != nil {
		t.Fatal(err)
	}

	var result mcpSearchOutput
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if len(result.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(result.Servers))
	}
	got := result.Servers[0]
	wantURL := server.URL + "/mcp-servers/s/ms1server"
	if !got.ConfigurationRequired || got.Status != "configuration required" || got.URL != wantURL {
		t.Fatalf("unexpected JSON result: %#v, want URL %q", got, wantURL)
	}
}

func TestMCPSearchEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(types.RegistryServerList{Servers: []types.RegistryServerResponse{}})
	}))
	defer server.Close()

	stdout, err := executeMCPTestCommand(t, mcpTestRoot(server.URL), "search")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "No MCP servers found") {
		t.Fatalf("expected empty message, got:\n%s", stdout)
	}
}

func TestMCPSearchRegistryAuthErrors(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{status: http.StatusUnauthorized, want: `registry search requires login; run "obot login" first`},
		{status: http.StatusForbidden, want: "authenticated user is not authorized to access the registry endpoint"},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "nope", tt.status)
			}))
			defer server.Close()

			_, err := executeMCPTestCommand(t, mcpTestRoot(server.URL), "search")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestMCPValidateCatalogYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entry.yaml")
	if err := os.WriteFile(path, []byte(`name: Test
entryKey: test
shortDescription: Test
description: Test
icon: icon
runtime: remote
remoteConfig:
  fixedURL: https://does-not-resolve.invalid/mcp
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.yaml"), []byte("not: a catalog entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ignoreobotcatalogs"), []byte("ignored.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-catalog-yaml", dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Catalog entries in 1 files are valid.") {
		t.Fatalf("unexpected output: %s", stdout)
	}
}

func TestMCPValidateCatalogYAMLRequiresEntryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entry.yaml")
	if err := os.WriteFile(path, []byte(`
- name: Missing
  shortDescription: Missing
  description: Missing
  icon: icon
  runtime: npx
  npxConfig:
    package: missing
- name: Whitespace
  entryKey: "  "
  shortDescription: Whitespace
  description: Whitespace
  icon: icon
  runtime: npx
  npxConfig:
    package: whitespace
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-catalog-yaml", "--require-entry-key", path)
	if err == nil || !strings.Contains(err.Error(), "entry.yaml[0]: entryKey is required") || !strings.Contains(err.Error(), "entry.yaml[1]: entryKey is required") {
		t.Fatalf("error = %v, want both missing entryKey errors", err)
	}
}

func TestMCPValidateCatalogYAMLSupportsEntryArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entries.yaml")
	if err := os.WriteFile(path, []byte(`
- name: First
  entryKey: first
  shortDescription: First
  description: First
  icon: icon
  runtime: npx
  npxConfig:
    package: first
- name: Second
  entryKey: second
  shortDescription: Second
  description: Second
  icon: icon
  runtime: uvx
  uvxConfig:
    package: second
`), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-catalog-yaml", path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "Catalog entries in 1 files are valid.") {
		t.Fatalf("unexpected output: %s", stdout)
	}
}

func TestMCPValidateCatalogYAMLAggregatesErrors(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"duplicate-key.yaml": `name: First
name: Second
runtime: npx`,
		"unknown-field.yaml": `name: Unknown
entryKey: shared
shortDescription: Unknown
description: Unknown
icon: icon
runtime: npx
npxConfig:
  package: test
unknownField: true`,
		"invalid-runtime.yaml": `name: Invalid
entryKey: shared
shortDescription: Invalid
description: Invalid
icon: icon
runtime: invalid`,
		"duplicate-entry-key.yaml": `name: Duplicate
entryKey: shared
shortDescription: Duplicate
description: Duplicate
icon: icon
runtime: npx
npxConfig:
  package: test`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-catalog-yaml", dir)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, expected := range []string{
		"duplicate-key.yaml",
		"key \"name\" already set",
		"unknown-field.yaml",
		"unknown field \"unknownField\"",
		"unsupported runtime",
		"duplicate source entry key \"shared\"",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %v, want %q", err, expected)
		}
	}
}

func TestMCPValidateSystemCatalogYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.yaml")
	if err := os.WriteFile(path, []byte(`
- name: Filter
  shortDescription: Filter
  description: Filter
  icon: icon
  systemMCPServerType: filter
  filterConfig:
    toolName: filter
  runtime: npx
  npxConfig:
    package: filter
- name: Remote
  shortDescription: Remote
  description: Remote
  icon: icon
  runtime: remote
  remoteConfig:
    fixedURL: https://does-not-resolve.invalid/mcp
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.yaml"), []byte("not: a system catalog entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ignoreobotcatalogs"), []byte("ignored.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-system-catalog-yaml", dir, path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "System catalog entries in 1 files are valid.") {
		t.Fatalf("unexpected output: %s", stdout)
	}
}

func TestMCPValidateSystemCatalogYAMLAggregatesErrors(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"unknown-field.yaml": `name: Unknown
shortDescription: Unknown
description: Unknown
icon: icon
runtime: npx
npxConfig:
  package: test
unknownField: true`,
		"missing-filter-config.yaml": `name: Missing Filter Config
shortDescription: Missing
description: Missing
icon: icon
systemMCPServerType: filter
runtime: npx
npxConfig:
  package: test`,
		"invalid-name.yaml": `name: "!!!"
shortDescription: Invalid
description: Invalid
icon: icon
runtime: npx
npxConfig:
  package: test`,
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-system-catalog-yaml", dir)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	for _, expected := range []string{
		"unknown-field.yaml",
		"unknown field \"unknownField\"",
		"missing-filter-config.yaml",
		"filterConfig is required",
		"invalid-name.yaml",
		"invalid system catalog entry name after sanitization",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("error = %v, want %q", err, expected)
		}
	}
}

func TestMCPValidateSystemCatalogYAMLRejectsDuplicateSanitizedNames(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.yaml")
	secondPath := filepath.Join(dir, "second.yaml")
	entry := func(name string) string {
		return fmt.Sprintf(`name: %q
shortDescription: Test
description: Test
icon: icon
runtime: npx
npxConfig:
  package: test
`, name)
	}
	if err := os.WriteFile(firstPath, []byte(entry("Shared Name")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte(entry("shared-name")), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "validate-system-catalog-yaml", firstPath, secondPath)
	if err == nil {
		t.Fatal("expected duplicate sanitized name error")
	}
	want := fmt.Sprintf("%s: duplicate sanitized system catalog entry name %q also used by %s", secondPath, "shared-name", firstPath)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func mcpTestRoot(baseURL string) *Obot {
	return &Obot{Client: &apiclient.Client{
		BaseURL: baseURL + "/api",
		Token:   "test-token",
	}}
}

func executeMCPTestCommand(t *testing.T, root *Obot, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := cmd.Command(&MCP{root: root})
	cmd.SetContext(t.Context())
	cmd.SetOut(&stdout)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), err
}

func registryTestServer(name, title, description, remoteURL string, configurationRequired bool) types.RegistryServerResponse {
	server := types.RegistryServerResponse{
		Server: types.RegistryServerDetail{
			Name:        name,
			Title:       title,
			Description: description,
		},
	}
	if remoteURL != "" {
		server.Server.Remotes = []types.RegistryServerRemote{{Type: "streamable-http", URL: remoteURL}}
	}
	if configurationRequired {
		server.Meta.Obot = &types.RegistryObotMeta{ConfigurationRequired: true}
	}
	return server
}
