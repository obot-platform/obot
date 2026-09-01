package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/version"
)

func TestConstructMCPServerMMMCPYAML(t *testing.T) {
	data, err := constructMCPServerMMMCPYAML(ServerConfig{
		MCPServerName:        "test-server",
		MCPServerDisplayName: "Test/Server",
		Command:              "npx",
		Args:                 []string{"example", "${LITERAL_ARG}"},
	}, map[string][]byte{
		"TOKEN": []byte("${LITERAL_TOKEN}"),
	})
	if err != nil {
		t.Fatalf("constructMCPServerMMMCPYAML() error = %v", err)
	}

	cfg, err := mmmcpconfig.Load(data, mmmcpconfig.LoadOptions{LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatalf("generated mmmcp config is invalid: %v\n%s", err, data)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("server count = %d, want 1", len(cfg.Servers))
	}
	if cfg.Name != "Test/Server" || cfg.Version != version.Get().String() {
		t.Fatalf("server identity = %q/%q, want %q/%q", cfg.Name, cfg.Version, "Test/Server", version.Get().String())
	}
	server := cfg.Servers[0]
	if server.Name != "Test-Server" || server.Command != "npx" {
		t.Fatalf("unexpected mmmcp server identity: %#v", server)
	}
	if len(server.Args) != 2 || server.Args[1] != "${LITERAL_ARG}" {
		t.Fatalf("args = %#v, want literal interpolation syntax preserved", server.Args)
	}
	if server.Env["TOKEN"] != "${LITERAL_TOKEN}" {
		t.Fatalf("TOKEN = %q, want literal interpolation syntax preserved", server.Env["TOKEN"])
	}
}

func TestMMMCPConfigPreservesCompositeSettings(t *testing.T) {
	config := MMMCPConfig(ServerConfig{
		Runtime:                types.RuntimeComposite,
		MCPServerName:          "composite-server",
		MCPServerDisplayName:   "Composite Server",
		PassthroughHeaderNames: []string{"X-Tenant"},
		Components: []ComponentServer{
			{
				DisplayName: "search",
				URL:         "http://127.0.0.1:8080/mcp-connect/component",
				ToolPrefix:  "custom",
				Tools: []types.ToolOverride{
					{
						Name:                "enabled",
						OverrideName:        "renamed",
						Description:         "original description",
						OverrideDescription: "replacement description",
						Enabled:             true,
					},
					{Name: "disabled", Enabled: false},
				},
			},
		},
	}, nil)

	if config.Name != "Composite Server" || config.Version != version.Get().String() {
		t.Fatalf("server identity = %q/%q, want %q/%q", config.Name, config.Version, "Composite Server", version.Get().String())
	}
	if len(config.Servers) != 1 {
		t.Fatalf("got %d component servers, want 1", len(config.Servers))
	}
	server := config.Servers[0]
	if server.Name != "search" || server.URL != "http://127.0.0.1:8080/mcp-connect/component" || server.Prefix != "custom" {
		t.Fatalf("unexpected component server: %#v", server)
	}
	if !slices.Equal(server.PassthroughHeaders, []string{"Authorization", "X-Tenant"}) {
		t.Fatalf("passthrough headers = %v", server.PassthroughHeaders)
	}
	if len(server.Tools) != 2 {
		t.Fatalf("tool overrides = %#v", server.Tools)
	}
	if tool := server.Tools[0]; tool.Name != "enabled" || tool.OverrideName != "renamed" || tool.Description != "original description" || tool.OverrideDescription != "replacement description" || !tool.Enabled {
		t.Fatalf("unexpected enabled tool override: %#v", tool)
	}
	if tool := server.Tools[1]; tool.Name != "disabled" || tool.Enabled {
		t.Fatalf("unexpected disabled tool override: %#v", tool)
	}
}

func TestMMMCPConfigServerNameFallsBackToName(t *testing.T) {
	config := MMMCPConfig(ServerConfig{
		MCPServerName: "fallback-server",
		URL:           "http://127.0.0.1:8080/mcp",
	}, nil)

	if config.Name != "fallback-server" || config.Version != version.Get().String() {
		t.Fatalf("server identity = %q/%q, want %q/%q", config.Name, config.Version, "fallback-server", version.Get().String())
	}
}

func TestServerHookConfigBuildsScopedHooks(t *testing.T) {
	hooks, servers := ServerHookConfig(ServerConfig{UserID: "user-1", Audiences: []string{"https://obot.example"}, Webhooks: []Webhook{
		{
			Name: "policy/resource", DisplayName: "Shared Display Name", URL: "https://policy.example/mcp", ToolName: "validate",
			Definitions: types.MCPSelectors{
				{Method: "tools/list"},
				{Method: "tools/call", Identifiers: []string{"echo"}},
				{Method: "resources/read", Identifiers: []string{"*"}},
			},
			MutateAllowed: true,
		},
	}})

	server, ok := servers["policy-resource"]
	if !ok {
		t.Fatalf("hook server did not use stable resource name: %#v", servers)
	}
	if server.MCPServerName != system.SystemMCPServerPrefix+"policy/resource" || server.URL != "https://policy.example/mcp" || server.UserID != "user-1" || !server.SystemMCPServer {
		t.Fatalf("unexpected native hook server config: %#v", server)
	}
	if len(server.Audiences) != 1 || server.Audiences[0] != "https://obot.example" {
		t.Fatalf("hook server did not retain audiences: %#v", server.Audiences)
	}
	if len(hooks) != 3 {
		t.Fatalf("got %d hook mappings, want 3: %#v", len(hooks), hooks)
	}
	if !hooks[0].Matches("tools/list", map[string]string{"name": "", "direction": "request"}) {
		t.Fatal("tools/list method selector did not match")
	}
	if !hooks[1].Matches("tools/call", map[string]string{"name": "echo"}) || hooks[1].Matches("tools/call", map[string]string{"name": "search"}) {
		t.Fatal("tools/call identifier selector did not scope by tool name")
	}
	if !hooks[2].Matches("resources/read", map[string]string{"name": "file:///readme"}) {
		t.Fatal("wildcard identifier did not match resource URI")
	}
	for _, hook := range hooks {
		if hook.Targets[0].Target != "policy-resource/validate" || hook.Targets[0].MutateDisallowed {
			t.Fatalf("unexpected hook target: %#v", hook.Targets[0])
		}
	}
}

func TestEnsureServerReadyUsesHealthzPath(t *testing.T) {
	var healthzCalls, mcpCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			healthzCalls++
			if r.Method != http.MethodGet {
				t.Fatalf("expected healthz check to use GET, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		case "/mcp":
			mcpCalls++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if err := ensureServerReady(ctx, server.URL, ServerConfig{
		Runtime:       types.RuntimeContainerized,
		ContainerPath: "/mcp",
		HealthzPath:   "/ready",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if healthzCalls != 1 {
		t.Fatalf("expected exactly one healthz call, got %d", healthzCalls)
	}
	if mcpCalls != 0 {
		t.Fatalf("expected MCP endpoint not to be probed, got %d calls", mcpCalls)
	}
}

func TestEnsureServerReadyHealthzPathWaitsForOK(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	if err := ensureServerReady(ctx, server.URL+"/", ServerConfig{HealthzPath: "healthz"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected two healthz calls, got %d", calls)
	}
}
