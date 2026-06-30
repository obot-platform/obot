package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oasdiff/yaml"
	"github.com/obot-platform/obot/apiclient/types"
)

func containsFold(s []string, v string) bool {
	for _, x := range s {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// TestCompositeNanobotYAMLForwardsAuthHeaders covers #6845: the composite
// nanobot config must forward the caller's auth to each child. Children are
// always internal Obot /mcp-connect endpoints, so Authorization is always
// included, plus any operator-configured passthrough header names — without
// duplicating Authorization if it was already configured.
func TestCompositeNanobotYAMLForwardsAuthHeaders(t *testing.T) {
	components := []ComponentServer{
		{Name: "child-a", URL: "http://obot/mcp-connect/child-a"},
		{Name: "child-b", URL: "http://obot/mcp-connect/child-b", ToolPrefix: "bq_"},
	}

	cases := []struct {
		name         string
		passthrough  []string
		wantContains []string
	}{
		{name: "default adds Authorization", passthrough: nil, wantContains: []string{"Authorization"}},
		{name: "configured names plus Authorization", passthrough: []string{"X-Tenant"}, wantContains: []string{"X-Tenant", "Authorization"}},
		{name: "no duplicate Authorization", passthrough: []string{"authorization"}, wantContains: []string{"authorization"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := constructMCPServerNanobotYAMLForComposite(components, tc.passthrough)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var cfg nanobotConfig
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("failed to unmarshal composite config: %v", err)
			}
			if len(cfg.MCPServers) != len(components) {
				t.Fatalf("expected %d child servers, got %d", len(components), len(cfg.MCPServers))
			}

			for name, child := range cfg.MCPServers {
				for _, want := range tc.wantContains {
					if !containsFold(child.PassthroughHeaders, want) {
						t.Fatalf("child %q passthrough %v missing %q", name, child.PassthroughHeaders, want)
					}
				}
				authCount := 0
				for _, h := range child.PassthroughHeaders {
					if strings.EqualFold(h, "Authorization") {
						authCount++
					}
				}
				if authCount != 1 {
					t.Fatalf("child %q expected exactly one Authorization, got %v", name, child.PassthroughHeaders)
				}
			}
		})
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
