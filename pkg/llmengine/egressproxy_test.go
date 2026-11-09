package llmengine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
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

func TestApplyEgressProxyCloudPoolSecondKey(t *testing.T) {
	for _, tc := range []struct {
		name   string
		family string
		scheme string
	}{
		{
			name:   "Azure HTTPS",
			family: "azure",
			scheme: "https",
		},
		{
			name:   "Azure HTTP",
			family: "azure",
			scheme: "http",
		},
		{
			name:   "Mantle HTTPS",
			family: "bedrock_mantle",
			scheme: "https",
		},
	} {
		for _, proxyURL := range []string{"http://egress.invalid:3128", "socks5://egress.invalid:1080"} {
			t.Run(tc.name+"/"+proxyURL, func(t *testing.T) {
				cfg := &httpproxy.Config{}
				if tc.scheme == "http" {
					cfg.HTTPProxy = proxyURL
				} else {
					cfg.HTTPSProxy = proxyURL
				}
				p := llmcoreclient.Provider{
					Config: schemas.ProviderConfig{
						NetworkConfig: schemas.NetworkConfig{DefaultRequestTimeoutInSeconds: 123},
					},
					Keys: []schemas.Key{{
						ID:    "first",
						Value: *schemas.NewSecretVar("first-secret"),
					}},
				}
				if tc.family == "azure" {
					p.Keys[0].AzureKeyConfig = &schemas.AzureKeyConfig{
						Endpoint: *schemas.NewSecretVar(tc.scheme + "://one.openai.azure.com"),
					}
				} else {
					p.Keys[0].BedrockMantleKeyConfig = &schemas.BedrockMantleKeyConfig{
						Region: schemas.NewSecretVar("eu-west-1"),
					}
				}
				require.True(t, applyProviderEgressProxy(tc.family, &p, cfg))
				require.Equal(t, proxyURL, p.Config.ProxyConfig.URL.GetValue())
				require.False(t, applyProviderEgressProxy(tc.family, &p, cfg), "single-key inheritance is idempotent")

				second := p.Keys[0]
				second.ID = "second"
				if tc.family == "azure" {
					second.AzureKeyConfig = &schemas.AzureKeyConfig{
						Endpoint: *schemas.NewSecretVar(tc.scheme + "://two.openai.azure.com"),
					}
				} else {
					second.BedrockMantleKeyConfig = &schemas.BedrockMantleKeyConfig{
						Region: schemas.NewSecretVar("us-east-1"),
					}
				}
				p.Keys = append(p.Keys, second)
				want := p
				want.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.EnvProxy}
				wantJSON := testJSON(t, want)
				require.True(t, applyProviderEgressProxy(tc.family, &p, cfg))
				require.JSONEq(t, string(wantJSON), string(testJSON(t, p)), "only the proxy may change when a second key is added")
				require.False(t, applyProviderEgressProxy(tc.family, &p, cfg), "shared-pool inheritance is idempotent")
			})
		}
	}
}

func TestApplyEgressProxyCloudPoolEndpointSchemes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		family   string
		endpoint string
		alias    string
		missing  bool
		wantEnv  bool
	}{
		{
			name:     "Azure HTTPS custom port",
			family:   "azure",
			endpoint: "https://cloud.invalid:8443",
		},
		{
			name:     "Azure HTTP on 443",
			family:   "azure",
			endpoint: "http://cloud.invalid:443",
		},
		{
			name:     "Azure explicit HTTPS port",
			family:   "azure",
			endpoint: "https://cloud.invalid:443",
			wantEnv:  true,
		},
		{
			name:     "Azure explicit HTTP port",
			family:   "azure",
			endpoint: "http://cloud.invalid:80",
			wantEnv:  true,
		},
		{
			name:     "Azure ordinary HTTP custom port",
			family:   "azure",
			endpoint: "http://cloud.invalid:8080",
			wantEnv:  true,
		},
		{
			name:     "Azure malformed endpoint",
			family:   "azure",
			endpoint: "https://cloud.invalid:bad",
		},
		{
			name:     "Azure endpoint without scheme",
			family:   "azure",
			endpoint: "cloud.invalid:8443",
		},
		{
			name:     "Azure unavailable endpoint port",
			family:   "azure",
			endpoint: "https://cloud.invalid:",
		},
		{
			name:   "Azure missing endpoint",
			family: "azure",
		},
		{
			name:    "Azure missing key config",
			family:  "azure",
			missing: true,
		},
		{
			name:     "Azure alias custom HTTPS port",
			family:   "azure",
			endpoint: "https://cloud.invalid",
			alias:    "https://alias.invalid:8443",
		},
		{
			name:     "Azure safe alias",
			family:   "azure",
			endpoint: "https://cloud.invalid",
			alias:    "https://alias.invalid:443",
			wantEnv:  true,
		},
		{
			name:     "Mantle endpoint custom port",
			family:   "bedrock_mantle",
			endpoint: "https://mantle.invalid:8443/v1",
		},
		{
			name:     "Mantle bare endpoint custom port",
			family:   "bedrock_mantle",
			endpoint: "mantle.invalid:8443",
		},
		{
			name:     "Mantle forces HTTPS despite pasted HTTP scheme",
			family:   "bedrock_mantle",
			endpoint: "http://mantle.invalid:80",
		},
		{
			name:     "Mantle explicit HTTPS port",
			family:   "bedrock_mantle",
			endpoint: "mantle.invalid:443",
			wantEnv:  true,
		},
		{
			name:     "Mantle malformed endpoint",
			family:   "bedrock_mantle",
			endpoint: "mantle.invalid:bad",
		},
		{
			name:    "Mantle unavailable key config",
			family:  "bedrock_mantle",
			missing: true,
		},
	} {
		for _, keyIndex := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/key%d", tc.name, keyIndex), func(t *testing.T) {
				cfg := &httpproxy.Config{HTTPSProxy: "http://egress.invalid:3128"}
				proxy := &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar(cfg.HTTPSProxy)}
				p := llmcoreclient.Provider{
					Config: schemas.ProviderConfig{ProxyConfig: proxy},
					Keys:   make([]schemas.Key, 2),
				}
				for i := range p.Keys {
					if tc.family == "azure" {
						p.Keys[i].AzureKeyConfig = &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar("https://safe.invalid")}
					} else {
						p.Keys[i].BedrockMantleKeyConfig = &schemas.BedrockMantleKeyConfig{Region: schemas.NewSecretVar("us-east-1")}
					}
				}
				key := &p.Keys[keyIndex]
				if tc.family == "azure" {
					key.AzureKeyConfig.Endpoint = *schemas.NewSecretVar(tc.endpoint)
					if tc.alias != "" {
						key.Aliases = schemas.KeyAliases{"alias": {AzureAliasCfg: &schemas.AzureAliasCfg{Endpoint: schemas.NewSecretVar(tc.alias)}}}
					}
				} else {
					key.BedrockMantleKeyConfig.Endpoints = &schemas.BedrockEndpoints{Mantle: schemas.NewSecretVar(tc.endpoint)}
				}
				if tc.missing {
					key.AzureKeyConfig = nil
					key.BedrockMantleKeyConfig = nil
				}
				before := testJSON(t, p)
				require.Equal(t, tc.wantEnv, applyProviderEgressProxy(tc.family, &p, cfg))
				if tc.wantEnv {
					require.Equal(t, schemas.EnvProxy, p.Config.ProxyConfig.Type)
				} else {
					require.Same(t, proxy, p.Config.ProxyConfig)
					require.JSONEq(t, string(before), string(testJSON(t, p)), "retention must leave the entire aggregate unchanged")
				}
				require.False(t, applyProviderEgressProxy(tc.family, &p, cfg), "retention and conversion must be idempotent")
				p.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.EnvProxy}
				require.False(t, applyProviderEgressProxy(tc.family, &p, cfg), "existing EnvProxy must remain unchanged even on unsafe endpoints")
			})
		}
	}
}

func TestApplyEgressProxyCloudPoolCustomHTTPSRouting(t *testing.T) {
	for _, httpTrap := range []bool{false, true} {
		t.Run(fmt.Sprintf("HTTP_PROXY_trap_%t", httpTrap), func(t *testing.T) {
			e, ctx := startTestEngine(t)
			var requests, connects, traps atomic.Int32
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.TLS == nil || r.Header.Get("api-key") != "key" {
					t.Error("expected TLS and the Azure API key at the upstream")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"chat-1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"proxied"},"finish_reason":"stop"}]}`))
			}))
			t.Cleanup(upstream.Close)
			upstreamURL, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			require.NotEqual(t, "443", upstreamURL.Port())
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodConnect {
					t.Error("HTTPS inference must use CONNECT")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				// Only the proxy maps this unresolvable destination to the local TLS upstream.
				if r.Host != "cloud.invalid:"+upstreamURL.Port() {
					t.Errorf("unexpected CONNECT target %q", r.Host)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				conn, dialErr := net.DialTimeout("tcp", upstreamURL.Host, 5*time.Second)
				if dialErr != nil {
					t.Error(dialErr)
					w.WriteHeader(http.StatusBadGateway)
					return
				}
				defer conn.Close()
				client, buffered, hijackErr := w.(http.Hijacker).Hijack()
				if hijackErr != nil {
					t.Error(hijackErr)
					return
				}
				defer client.Close()
				if _, writeErr := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); writeErr != nil {
					t.Error(writeErr)
					return
				}
				connects.Add(1)
				go func() { _, _ = io.Copy(conn, buffered) }()
				_, _ = io.Copy(client, conn)
			}))
			t.Cleanup(proxy.Close)
			trap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				traps.Add(1)
				w.WriteHeader(http.StatusBadGateway)
			}))
			t.Cleanup(trap.Close)
			for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy"} {
				t.Setenv(name, "")
			}
			t.Setenv("HTTPS_PROXY", proxy.URL)
			if httpTrap {
				t.Setenv("HTTP_PROXY", trap.URL)
			}
			p := llmcoreclient.Provider{}
			p.Config.CheckAndSetDefaults()
			p.Config.NetworkConfig.MaxRetries = 0
			// The local TLS target uses a self-signed certificate; proxy settings stay inherited.
			p.Config.NetworkConfig.InsecureSkipVerify = true
			p.Keys = []schemas.Key{{
				ID:             "first",
				Name:           "first",
				Value:          *schemas.NewSecretVar("key"),
				Models:         schemas.WhiteList{"*"},
				Weight:         1,
				AzureKeyConfig: &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar("https://cloud.invalid:" + upstreamURL.Port())},
			}}
			require.NoError(t, e.store.CreateProvider(ctx, "azure", p))
			require.True(t, e.ApplyEgressProxy(ctx))
			stored, err := e.store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			inherited := stored.Config.ProxyConfig
			require.Equal(t, proxy.URL, inherited.URL.GetValue())
			second := p.Keys[0]
			second.ID, second.Name = "second", "second"
			require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
				p.Keys = append(p.Keys, second)
				return nil
			}))
			changed := e.ApplyEgressProxy(ctx)
			_, err = e.Sync(ctx)
			require.NoError(t, err)
			for _, id := range []string{"first", "second"} {
				bctx, cancel := schemas.NewBifrostContextWithCancel(ctx)
				bctx.SetValue(schemas.BifrostContextKeyAPIKeyID, id)
				response, requestErr := e.Client().ChatCompletionRequest(bctx, &schemas.BifrostChatRequest{
					Provider: schemas.Azure,
					Model:    "gpt-4o",
					Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
				})
				cancel()
				if requestErr != nil {
					t.Logf("routing failure: %v; HTTPS proxy CONNECTs=%d, HTTP proxy trap=%d, upstream requests=%d", requestErr.Error, connects.Load(), traps.Load(), requests.Load())
				}
				require.Nil(t, requestErr, "HTTPS on a non443 port must still reach HTTPS_PROXY")
				require.NotNil(t, response)
			}
			require.Equal(t, int32(2), requests.Load())
			require.Positive(t, connects.Load(), "actual TLS requests must traverse the CONNECT proxy")
			require.Zero(t, traps.Load(), "HTTP_PROXY must not receive HTTPS inference")
			require.False(t, changed)
			require.False(t, e.ApplyEgressProxy(ctx), "retained fixed proxy must be idempotent")
			stored, err = e.store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			require.Equal(t, inherited, stored.Config.ProxyConfig)
		})
	}
}

func TestApplyEgressProxyPreservesExplicitProxySettings(t *testing.T) {
	const environmentProxy = "http://egress.invalid:3128"
	for _, tc := range []struct {
		name  string
		proxy schemas.ProxyConfig
	}{
		{
			name: "unrelated HTTP proxy",
			proxy: schemas.ProxyConfig{
				Type: schemas.HTTPProxy,
				URL:  schemas.NewSecretVar("http://admin.invalid:3128"),
			},
		},
		{
			name: "unrelated SOCKS proxy",
			proxy: schemas.ProxyConfig{
				Type:     schemas.Socks5Proxy,
				URL:      schemas.NewSecretVar("socks5://admin.invalid:1080"),
				Username: schemas.NewSecretVar("admin"),
				Password: schemas.NewSecretVar("secret"),
			},
		},
		{
			name: "matching URL with username",
			proxy: schemas.ProxyConfig{
				Type:     schemas.HTTPProxy,
				URL:      schemas.NewSecretVar(environmentProxy),
				Username: schemas.NewSecretVar("admin"),
			},
		},
		{
			name: "matching URL with password",
			proxy: schemas.ProxyConfig{
				Type:     schemas.HTTPProxy,
				URL:      schemas.NewSecretVar(environmentProxy),
				Password: schemas.NewSecretVar("secret"),
			},
		},
		{
			name: "matching URL with CA certificate",
			proxy: schemas.ProxyConfig{
				Type:      schemas.HTTPProxy,
				URL:       schemas.NewSecretVar(environmentProxy),
				CACertPEM: schemas.NewSecretVar("admin-ca"),
			},
		},
		{
			name: "matching URL with bypass list",
			proxy: schemas.ProxyConfig{
				Type:    schemas.HTTPProxy,
				URL:     schemas.NewSecretVar(environmentProxy),
				NoProxy: "admin.internal",
			},
		},
		{
			name: "matching URL with TLS option",
			proxy: schemas.ProxyConfig{
				Type:          schemas.HTTPProxy,
				URL:           schemas.NewSecretVar(environmentProxy),
				SkipTLSVerify: true,
			},
		},
		{
			name: "existing environment proxy",
			proxy: schemas.ProxyConfig{
				Type: schemas.EnvProxy,
				URL:  schemas.NewSecretVar(environmentProxy),
			},
		},
		{
			name: "unknown proxy type",
			proxy: schemas.ProxyConfig{
				Type: "unknown",
				URL:  schemas.NewSecretVar(environmentProxy),
			},
		},
	} {
		for _, family := range []string{"azure", "bedrock_mantle"} {
			for _, keyCount := range []int{1, 2} {
				t.Run(fmt.Sprintf("%s/%s/%d", tc.name, family, keyCount), func(t *testing.T) {
					proxy := tc.proxy
					p := llmcoreclient.Provider{
						Config: schemas.ProviderConfig{ProxyConfig: &proxy},
						Keys:   make([]schemas.Key, keyCount),
					}
					cfg := &httpproxy.Config{
						HTTPSProxy: environmentProxy,
						NoProxy:    "*",
					}
					require.False(t, applyProviderEgressProxy(family, &p, cfg))
					require.Same(t, &proxy, p.Config.ProxyConfig)
					require.Equal(t, tc.proxy, *p.Config.ProxyConfig)
				})
			}
		}
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
