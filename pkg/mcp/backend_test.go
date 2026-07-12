package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oasdiff/yaml"
	"github.com/obot-platform/obot/apiclient/types"
)

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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := ensureServerReady(ctx, server.URL+"/", ServerConfig{HealthzPath: "healthz"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected two healthz calls, got %d", calls)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositeIncludesOnlyEnabledTools(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite([]ComponentServer{
		{
			Name:       "configured-ping-echo",
			URL:        "https://example.com/mcp",
			ToolPrefix: "configured_",
			Tools: []types.ToolOverride{
				{Name: "ping", Enabled: false},
				{Name: "echo", Enabled: true},
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
	data, err := constructMCPServerNanobotYAMLForComposite([]ComponentServer{
		{
			Name: "default-tools",
			URL:  "https://example.com/mcp",
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
	if server.ToolOverrides != nil {
		t.Fatalf("expected omitted overrides not to set toolOverrides, got %#v", server.ToolOverrides)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositePreservesComponentsWithNoEnabledTools(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite([]ComponentServer{
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

func mustUnmarshalNanobotConfig(t *testing.T, data []byte) nanobotConfig {
	t.Helper()
	var config nanobotConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("failed to unmarshal nanobot config: %v\n%s", err, string(data))
	}
	return config
}
