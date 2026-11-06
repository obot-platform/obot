package llmengine

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http/httpproxy"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestApplyEgressProxy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	e := New()
	if err := e.Start(ctx, testOptions(t, Options{DataDir: t.TempDir()})); err != nil {
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
		if err := testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, p))); err != nil {
			t.Fatal(err)
		}
	}

	e.ApplyEgressProxy(ctx)

	var resp struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := testResult(&resp, "providers")(e.ListProviders(ctx)); err != nil {
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
	if err := testResult(&custom, "")(e.GetProvider(ctx, "generic-responses")); err != nil {
		t.Fatal(err)
	}
	delete(custom, "keys")
	custom["network_config"].(map[string]any)["base_url"] = "http://127.0.0.1:9/v1"
	if err := testMutation(ctx, e, nil)(e.UpdateProvider(ctx, "generic-responses", testJSON(t, custom), false)); err != nil {
		t.Fatal(err)
	}
	e.ApplyEgressProxy(ctx)
	updated, err := e.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	if updated.ProxyConfig != nil && updated.ProxyConfig.Type != schemas.NoProxy {
		t.Errorf("a provider moved to localhost still uses the egress proxy: %v", updated.ProxyConfig)
	}
}

func TestApplyEgressProxyPreservesConcurrentEdits(t *testing.T) {
	const environmentProxy = "http://proxy.example:3128"
	for _, tc := range []struct {
		name         string
		initialURL   string
		latestURL    string
		initialProxy string
		latestProxy  string
		wantProxy    string
		wantChanged  bool
		conflict     bool
	}{
		{
			name:        "add proxy without overwriting newer settings",
			initialURL:  "https://api.openai.com",
			wantProxy:   environmentProxy,
			wantChanged: true,
		},
		{
			name:         "remove proxy without overwriting newer settings",
			initialURL:   "http://127.0.0.1:9",
			initialProxy: environmentProxy,
			wantChanged:  true,
		},
		{
			name:       "new local endpoint no longer needs proxy",
			initialURL: "https://api.openai.com",
			latestURL:  "http://127.0.0.1:9",
		},
		{
			name:         "new public endpoint still needs proxy",
			initialURL:   "http://127.0.0.1:9",
			latestURL:    "https://api.openai.com",
			initialProxy: environmentProxy,
			wantProxy:    environmentProxy,
		},
		{
			name:        "new administrator proxy is preserved",
			initialURL:  "https://api.openai.com",
			latestProxy: "http://admin-proxy.example:8080",
			wantProxy:   "http://admin-proxy.example:8080",
		},
		{
			name:         "new administrator proxy is not removed on a local endpoint",
			initialURL:   "http://127.0.0.1:9",
			initialProxy: environmentProxy,
			latestProxy:  "http://admin-proxy.example:3128",
			wantProxy:    "http://admin-proxy.example:3128",
		},
		{
			name:       "write conflict preserves the winning edit",
			initialURL: "https://api.openai.com",
			conflict:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			base, err := llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			var armed, injected bool
			var writes int
			var latest llmcoreclient.Provider
			wrapped := interceptor.NewClient(storage, interceptor.Funcs{
				List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
					if err := c.List(ctx, list, opts...); err != nil {
						return err
					}
					if _, ok := list.(*v1.LLMGatewayProviderList); !ok || !armed || injected {
						return nil
					}
					// Publish an edit after capturing the listing, before its consumer mutates.
					injected = true
					err := base.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
						p.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = 901
						p.Keys[0].Value = *schemas.NewSecretVar("new-key-secret")
						if tc.latestURL != "" {
							p.Config.NetworkConfig.BaseURL = tc.latestURL
						}
						if tc.latestProxy != "" {
							p.Config.ProxyConfig = &schemas.ProxyConfig{
								Type: schemas.HTTPProxy,
								URL:  schemas.NewSecretVar(tc.latestProxy),
							}
						}
						return nil
					})
					if err == nil {
						latest, err = base.GetProvider(ctx, "openai")
					}
					return err
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok && armed {
						writes++
						if tc.conflict {
							// A later edit wins between the mutation's read and write.
							if err := base.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
								p.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = 902
								return nil
							}); err != nil {
								return err
							}
							var err error
							latest, err = base.GetProvider(ctx, "openai")
							if err != nil {
								return err
							}
						}
					}
					return c.Update(ctx, obj, opts...)
				},
			})
			store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
			require.NoError(t, err)
			engine := New()
			require.NoError(t, engine.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
			t.Setenv("HTTPS_PROXY", environmentProxy)
			t.Setenv("HTTP_PROXY", "")
			t.Setenv("NO_PROXY", "")
			t.Setenv("https_proxy", "")
			t.Setenv("http_proxy", "")
			t.Setenv("no_proxy", "")
			provider := llmcoreclient.Provider{
				Config: schemas.ProviderConfig{
					NetworkConfig: schemas.NetworkConfig{
						BaseURL:                        tc.initialURL,
						DefaultRequestTimeoutInSeconds: 300,
					},
				},
				Keys: []schemas.Key{{
					ID:    "key",
					Value: *schemas.NewSecretVar("old-key-secret"),
				}},
			}
			if tc.initialProxy != "" {
				provider.Config.ProxyConfig = &schemas.ProxyConfig{
					Type: schemas.HTTPProxy,
					URL:  schemas.NewSecretVar(tc.initialProxy),
				}
			}
			require.NoError(t, base.CreateProvider(t.Context(), "openai", provider))
			armed = true

			require.Equal(t, tc.wantChanged, engine.ApplyEgressProxy(t.Context()))
			require.True(t, injected)
			if tc.conflict {
				require.Equal(t, 1, writes, "a conflicted mutation must not report success or retry a stale write")
			}
			got, err := base.GetProvider(t.Context(), "openai")
			require.NoError(t, err)
			if tc.wantProxy == "" {
				require.True(t, got.Config.ProxyConfig == nil || got.Config.ProxyConfig.Type == schemas.NoProxy)
			} else {
				require.NotNil(t, got.Config.ProxyConfig)
				require.Equal(t, tc.wantProxy, got.Config.ProxyConfig.URL.GetValue())
			}
			got.Config.ProxyConfig = latest.Config.ProxyConfig
			require.Equal(t, latest, got, "only proxy settings may change after the concurrent edit")
		})
	}
}

func TestIsEgressProxy(t *testing.T) {
	cfg := &httpproxy.Config{HTTPSProxy: "http://user:pw@proxy.example:3128"}
	for _, tt := range []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "the egress proxy",
			url:  cfg.HTTPSProxy,
			want: true,
		},
		{
			name: "different proxy with the same redacted prefix and suffix",
			url:  "http://admin:other@different.example:3128",
		},
		{
			name: "an admin's proxy",
			url:  "http://corp-proxy:8080",
		},
		{
			name: "no URL",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxy := &schemas.ProxyConfig{
				Type: schemas.HTTPProxy,
				URL:  schemas.NewSecretVar(tt.url),
			}
			require.Equal(t, tt.want, isEgressProxy(proxy, cfg))
		})
	}
	require.False(t, isEgressProxy(nil, cfg))
}
