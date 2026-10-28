package llmengine

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http/httpproxy"
)

func TestApplyEgressProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	e := New()
	if err := e.Start(ctx, Options{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	// The engine gives new providers the proxy the process started with, if any; otherwise
	// Obot applies one. Set after starting: the engine's own startup downloads would otherwise
	// use it.
	if os.Getenv("HTTPS_PROXY") == "" {
		t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
		t.Setenv("HTTP_PROXY", "")
		t.Setenv("NO_PROXY", "")
	}
	// A built-in provider, and a custom one like the legacy import creates.
	for _, p := range []map[string]any{
		{"provider": "openai"},
		{
			"provider":               "generic-responses",
			"custom_provider_config": map[string]any{"base_provider_type": "openai"},
			"network_config":         map[string]any{"base_url": "https://10.255.255.1/v1", "allow_private_network": true, "default_request_timeout_in_seconds": 300},
		},
	} {
		if err := e.Do(ctx, http.MethodPost, "/api/providers", p, nil); err != nil {
			t.Fatal(err)
		}
	}

	e.ApplyEgressProxy(ctx)

	var resp struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, p := range resp.Providers {
		name, _ := p["name"].(string)
		if name != "openai" && name != "generic-responses" {
			continue
		}
		seen++
		if p["proxy_config"] == nil {
			t.Errorf("provider %s did not get the egress proxy", name)
		}
		if name == "generic-responses" && p["custom_provider_config"] == nil {
			t.Errorf("the custom provider lost its configuration")
		}
	}
	if seen != 2 {
		t.Fatalf("expected both providers, got %v", resp.Providers)
	}

	// Moving a provider to a local endpoint, with its proxy sent back as the UI does, removes
	// the egress proxy Obot gave it.
	var custom map[string]any
	if err := e.Do(ctx, http.MethodGet, "/api/providers/generic-responses", nil, &custom); err != nil {
		t.Fatal(err)
	}
	delete(custom, "keys")
	custom["network_config"].(map[string]any)["base_url"] = "http://127.0.0.1:9/v1"
	if err := e.Do(ctx, http.MethodPut, "/api/providers/generic-responses", custom, nil); err != nil {
		t.Fatal(err)
	}
	e.ApplyEgressProxy(ctx)
	if err := e.Do(ctx, http.MethodGet, "/api/providers/generic-responses", nil, &custom); err != nil {
		t.Fatal(err)
	}
	if hasProxy(custom["proxy_config"]) {
		t.Errorf("a provider moved to localhost still uses the egress proxy: %v", custom["proxy_config"])
	}
}

func TestIsEgressProxy(t *testing.T) {
	cfg := &httpproxy.Config{HTTPSProxy: "http://user:pw@proxy.example:3128"}
	for _, tt := range []struct {
		name   string
		config any
		want   bool
	}{
		{name: "the egress proxy", config: map[string]any{"type": "http", "url": map[string]any{"value": "http://user:pw@proxy.example:3128"}}, want: true},
		{name: "the egress proxy, redacted", config: map[string]any{"url": map[string]any{"value": "http" + strings.Repeat("*", 24) + "3128"}}, want: true},
		{name: "another proxy, redacted", config: map[string]any{"url": map[string]any{"value": "http" + strings.Repeat("*", 24) + "8080"}}},
		{name: "an admin's proxy", config: map[string]any{"url": map[string]any{"value": "http://corp-proxy:8080"}}},
		{name: "no URL", config: map[string]any{"type": "http"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEgressProxy(tt.config, cfg); got != tt.want {
				t.Errorf("isEgressProxy = %v, want %v", got, tt.want)
			}
		})
	}
}
