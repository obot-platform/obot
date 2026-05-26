package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	stdout, err := executeMCPTestCommand(mcpTestRoot(server.URL), "search", "github", "issues", "--limit", "3")
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

	stdout, err := executeMCPTestCommand(mcpTestRoot(server.URL), "search", "--json")
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

	stdout, err := executeMCPTestCommand(mcpTestRoot(server.URL), "search", "--json")
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

	stdout, err := executeMCPTestCommand(mcpTestRoot(server.URL), "search")
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

			_, err := executeMCPTestCommand(mcpTestRoot(server.URL), "search")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func mcpTestRoot(baseURL string) *Obot {
	return &Obot{Client: &apiclient.Client{
		BaseURL: baseURL + "/api",
		Token:   "test-token",
	}}
}

func executeMCPTestCommand(root *Obot, args ...string) (string, error) {
	var stdout bytes.Buffer
	cmd := cmd.Command(&MCP{root: root})
	cmd.SetContext(context.Background())
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
