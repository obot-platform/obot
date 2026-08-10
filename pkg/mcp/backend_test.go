package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oasdiff/yaml"
	ntypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
)

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

func TestConstructMCPServerNanobotYAMLForCompositeIncludesOnlyEnabledTools(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite(ServerConfig{
		Components: []ComponentServer{
			{
				Name:       "configured-ping-echo",
				URL:        "https://example.com/mcp",
				ToolPrefix: "configured_",
				Tools: []types.ToolOverride{
					{Name: "ping", Enabled: false},
					{Name: "echo", Enabled: true},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config := mustUnmarshalNanobotConfig(t, data)
	server := config.MCPServers["configured-ping-echo"]
	if server.ToolPrefix != "configured_" {
		t.Fatalf("expected toolPrefix configured_, got %q", server.ToolPrefix)
	}
	if server.NoTools {
		t.Fatal("expected noTools to be false")
	}
	if len(server.ToolOverrides) != 1 {
		t.Fatalf("expected one tool override, got %#v", server.ToolOverrides)
	}
	if _, ok := server.ToolOverrides["echo"]; !ok {
		t.Fatalf("expected echo to be included, got %#v", server.ToolOverrides)
	}
	if _, ok := server.ToolOverrides["ping"]; ok {
		t.Fatalf("expected ping to be omitted, got %#v", server.ToolOverrides)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositeOmitsToolConfigWhenOverridesOmitted(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite(ServerConfig{
		Components: []ComponentServer{
			{
				Name: "default-tools",
				URL:  "https://example.com/mcp",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config := mustUnmarshalNanobotConfig(t, data)
	server := config.MCPServers["default-tools"]
	if server.NoTools {
		t.Fatal("expected omitted overrides not to set noTools")
	}
	if strings.Contains(string(data), "toolOverrides") {
		t.Fatalf("expected omitted overrides not to set toolOverrides, got YAML:\n%s", string(data))
	}
	if len(server.ToolOverrides) != 0 {
		t.Fatalf("expected omitted overrides not to set toolOverrides, got %#v", server.ToolOverrides)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositePreservesComponentsWithNoEnabledTools(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite(ServerConfig{
		Components: []ComponentServer{
			{
				Name:    "ping-echo",
				URL:     "https://example.com/mcp",
				Tools:   []types.ToolOverride{},
				noTools: true,
			},
			{
				Name:       "configured-ping-echo",
				URL:        "https://example.com/configured-mcp",
				ToolPrefix: "configured_",
				Tools: []types.ToolOverride{
					{Name: "ping", Enabled: false},
					{Name: "echo", Enabled: true},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config := mustUnmarshalNanobotConfig(t, data)

	disabledOnlyServer := config.MCPServers["ping-echo"]
	if !disabledOnlyServer.NoTools {
		t.Fatal("expected component with no enabled tools to set noTools")
	}
	if len(disabledOnlyServer.ToolOverrides) != 0 {
		t.Fatalf("expected no enabled tool overrides, got %#v", disabledOnlyServer.ToolOverrides)
	}

	configuredServer := config.MCPServers["configured-ping-echo"]
	if configuredServer.ToolPrefix != "configured_" {
		t.Fatalf("expected toolPrefix configured_, got %q", configuredServer.ToolPrefix)
	}
	if configuredServer.NoTools {
		t.Fatal("expected configured component to expose enabled tools")
	}
	if len(configuredServer.ToolOverrides) != 1 {
		t.Fatalf("expected one configured tool override, got %#v", configuredServer.ToolOverrides)
	}
	if _, ok := configuredServer.ToolOverrides["echo"]; !ok {
		t.Fatalf("expected echo to be included, got %#v", configuredServer.ToolOverrides)
	}
	if _, ok := configuredServer.ToolOverrides["ping"]; ok {
		t.Fatalf("expected ping to be omitted, got %#v", configuredServer.ToolOverrides)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositeOmitsWebhooks(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite(ServerConfig{
		Components: []ComponentServer{
			{
				Name: "component",
				URL:  "https://example.com/mcp",
			},
		},
		Webhooks: []Webhook{
			{
				Name:        "fallback-webhook",
				DisplayName: "review/webhook",
				URL:         "https://example.com/webhook",
				ToolName:    "validate",
				Definitions: types.MCPSelectors{
					{Method: "tools/call", Identifiers: []string{"echo"}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config := mustUnmarshalNanobotConfig(t, data)

	if _, ok := config.MCPServers["review-webhook"]; ok {
		t.Fatalf("expected webhook server to be omitted, got %#v", config.MCPServers)
	}
	if len(config.Hooks) != 0 {
		t.Fatalf("expected hook mappings to be omitted, got %#v", config.Hooks)
	}
}

func mustUnmarshalNanobotConfig(t *testing.T, data []byte) ntypes.Config {
	t.Helper()
	var config ntypes.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal nanobot config: %v\n%s", err, string(data))
	}
	return config
}
