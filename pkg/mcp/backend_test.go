package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	yaml := string(data)
	for _, expected := range []string{"toolOverrides:", "echo:", "toolPrefix: configured_"} {
		if !strings.Contains(yaml, expected) {
			t.Fatalf("expected YAML to contain %q, got:\n%s", expected, yaml)
		}
	}
	if strings.Contains(yaml, "\n            ping:") {
		t.Fatalf("expected disabled tool to be omitted, got:\n%s", yaml)
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

	yaml := string(data)
	if strings.Contains(yaml, "noTools") || strings.Contains(yaml, "toolOverrides") {
		t.Fatalf("expected omitted overrides to expose all tools, got:\n%s", yaml)
	}
}

func TestConstructMCPServerNanobotYAMLForCompositePreservesComponentsWithNoEnabledTools(t *testing.T) {
	data, err := constructMCPServerNanobotYAMLForComposite([]ComponentServer{
		{
			Name: "ping-echo",
			URL:  "https://example.com/mcp",
			Tools: []types.ToolOverride{
				{Name: "echo", OverrideName: "repeat", OverrideDescription: "Repeat a message", Enabled: false},
			},
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

	yaml := string(data)
	for _, expected := range []string{"\n    ping-echo:", "noTools: true", "configured-ping-echo:", "toolPrefix: configured_", "echo:"} {
		if !strings.Contains(yaml, expected) {
			t.Fatalf("expected YAML to contain %q, got:\n%s", expected, yaml)
		}
	}
	for _, unexpected := range []string{"repeat", "\n            ping:", "noTools: false"} {
		if strings.Contains(yaml, unexpected) {
			t.Fatalf("expected YAML not to contain %q, got:\n%s", unexpected, yaml)
		}
	}
}
