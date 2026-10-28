package llmengine

import (
	"context"
	"maps"
	"net/http"
	"testing"
	"time"
)

// A caller's Claude subscription token may only be forwarded when the Anthropic provider
// sends straight to Anthropic over verified TLS.
func TestAnthropicIsDirect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	e := New()
	if err := e.Start(ctx, Options{DataDir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if e.AnthropicIsDirect(ctx) {
		t.Fatal("direct without an Anthropic provider")
	}
	if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "anthropic"}, nil); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		networkConfig map[string]any
		proxyConfig   map[string]any
		globalProxy   map[string]any
		want          bool
	}{
		{
			name:          "default base URL",
			networkConfig: map[string]any{},
			want:          true,
		},
		{
			name:          "Anthropic's own URL",
			networkConfig: map[string]any{"base_url": "https://api.anthropic.com"},
			want:          true,
		},
		{
			name:          "another host",
			networkConfig: map[string]any{"base_url": "https://127.0.0.1:8443", "allow_private_network": true},
		},
		{
			name:          "Anthropic's host over plain HTTP",
			networkConfig: map[string]any{"base_url": "http://api.anthropic.com"},
		},
		{
			name:          "certificate checks off",
			networkConfig: map[string]any{"insecure_skip_verify": true},
		},
		{
			name:          "another CA trusted",
			networkConfig: map[string]any{"ca_cert_pem": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"},
		},
		{
			name:          "a proxy that only relays, such as the egress proxy",
			networkConfig: map[string]any{},
			proxyConfig:   map[string]any{"type": "http", "url": map[string]any{"value": "http://proxy.corp:3128", "type": "plain_text"}},
			want:          true,
		},
		{
			name:          "another CA trusted through the proxy",
			networkConfig: map[string]any{},
			proxyConfig:   map[string]any{"type": "http", "url": "http://proxy.corp:3128", "ca_cert_pem": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"},
		},
		{
			name:          "the global proxy with certificate checks on",
			networkConfig: map[string]any{},
			globalProxy:   map[string]any{"enabled": true, "type": "http", "url": "http://127.0.0.1:3128", "enable_for_inference": true},
			want:          true,
		},
		{
			name:          "the global proxy with certificate checks off",
			networkConfig: map[string]any{},
			globalProxy:   map[string]any{"enabled": true, "type": "http", "url": "https://127.0.0.1:3128", "skip_tls_verify": true, "enable_for_inference": true},
		},
		{
			name:          "the global proxy disabled with certificate checks off",
			networkConfig: map[string]any{},
			globalProxy:   map[string]any{"enabled": false, "type": "http", "skip_tls_verify": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var provider map[string]any
			if err := e.Do(ctx, http.MethodGet, "/api/providers/anthropic", nil, &provider); err != nil {
				t.Fatal(err)
			}
			nc := map[string]any{"default_request_timeout_in_seconds": 30}
			maps.Copy(nc, tt.networkConfig)
			proxy := tt.proxyConfig
			if proxy == nil {
				proxy = map[string]any{"type": "none"}
			}
			update := map[string]any{"network_config": nc, "proxy_config": proxy}
			for _, k := range []string{"keys", "concurrency_and_buffer_size", "custom_provider_config"} {
				if v, ok := provider[k]; ok {
					update[k] = v
				}
			}
			if err := e.Do(ctx, http.MethodPut, "/api/providers/anthropic", update, nil); err != nil {
				t.Fatal(err)
			}
			globalProxy := tt.globalProxy
			if globalProxy == nil {
				globalProxy = map[string]any{"enabled": false, "type": "http"}
			}
			if err := e.Do(ctx, http.MethodPut, "/api/proxy-config", globalProxy, nil); err != nil {
				t.Fatal(err)
			}
			if got := e.AnthropicIsDirect(ctx); got != tt.want {
				t.Fatalf("AnthropicIsDirect = %v, want %v", got, tt.want)
			}
		})
	}
}
