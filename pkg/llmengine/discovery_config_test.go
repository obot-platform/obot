package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type listingPause struct {
	entered chan struct{}
	release chan struct{}
}

type listingBarrier struct {
	pre  atomic.Pointer[listingPause]
	post atomic.Pointer[listingPause]
}

func newListingPause(t *testing.T) *listingPause {
	t.Helper()
	p := &listingPause{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(sync.OnceFunc(func() {
		select {
		case <-p.release:
		default:
			close(p.release)
		}
	}))
	return p
}

func (p *listingPause) wait(t *testing.T) {
	t.Helper()
	select {
	case <-p.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("listing did not reach barrier")
	}
}

func (p *listingPause) block(ctx context.Context) {
	close(p.entered)
	select {
	case <-p.release:
	case <-ctx.Done():
	}
}

func (*listingBarrier) GetName() string { return "test-listing-barrier" }
func (*listingBarrier) Cleanup() error  { return nil }
func (*listingBarrier) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}
func (b *listingBarrier) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req.ListModelsRequest != nil && req.ListModelsRequest.Provider == schemas.OpenAI {
		if p := b.pre.Swap(nil); p != nil {
			p.block(ctx)
		}
	}
	return req, nil, nil
}
func (b *listingBarrier) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if resp != nil && resp.ListModelsResponse != nil {
		if p := b.post.Swap(nil); p != nil {
			p.block(ctx)
		}
	}
	return resp, err, nil
}

func directDiscoveryListing(ctx context.Context, e *Engine) *schemas.BifrostError {
	bctx, cancel := schemas.NewBifrostContextWithCancel(ctx)
	defer cancel()
	bctx.SetValue(schemas.BifrostContextKeyAllowPerRequestRawOverride, true)
	bctx.SetValue(schemas.BifrostContextKeySendBackRawResponse, true)
	_, err := e.Client().ListModelsRequest(bctx, &schemas.BifrostListModelsRequest{Provider: schemas.OpenAI})
	return err
}

func TestModelDiscoveryKeyRotationBetweenHooksAndWorker(t *testing.T) {
	barrier := &listingBarrier{}
	e, _ := migrationHealthFixture(t, interceptor.Funcs{}, barrier)
	response := newListingPause(t)
	auth := make(chan string, 1)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth <- r.Header.Get("Authorization")
		response.block(r.Context())
		_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
	}))
	t.Cleanup(func() {
		select {
		case <-response.release:
		default:
			close(response.release)
		}
		up.Close()
	})
	set := func(key string) {
		t.Helper()
		require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
			p.Config.NetworkConfig.BaseURL = up.URL
			p.Keys[0].Value = *schemas.NewSecretVar(key)
			return nil
		}))
		_, err := e.Sync(t.Context())
		require.NoError(t, err)
	}
	set("key-A")
	beforeWorker := newListingPause(t)
	barrier.pre.Store(beforeWorker)
	done := make(chan *schemas.BifrostError, 1)
	go func() { done <- directDiscoveryListing(t.Context(), e) }()
	beforeWorker.wait(t)
	set("key-B")
	close(beforeWorker.release)
	response.wait(t)
	require.Equal(t, "Bearer key-B", <-auth, "worker selects keys after PreLLMHook")
	set("key-A")
	cfg, err := e.config(t.Context())
	require.NoError(t, err)
	require.True(t, e.core.Load().ProviderApplied(schemas.OpenAI, cfg))
	close(response.release)
	select {
	case err := <-done:
		require.Nil(t, err, "ordinary listing must remain usable")
	case <-time.After(5 * time.Second):
		t.Fatal("listing did not finish")
	}
	require.NotContains(t, e.availability.discoveredModels(cfg), "openai/openai", "A->B->A checks at hooks cannot prove the worker used A")
}

func TestModelDiscoveryIdentity(t *testing.T) {
	fixture := func() llmcoreclient.Config {
		key := azureTestKey("key")
		key.Value = *schemas.NewSecretVar("credential")
		key.AzureKeyConfig.ClientSecret = schemas.NewSecretVar("env.OBOT_DISCOVERY_TEST_SECRET")
		key.AzureKeyConfig.ClientSecret.Val = "resolved-A"
		pc := schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: "https://provider.example"}}
		pc.NetworkConfig.CACertPEM = schemas.NewSecretVar("env.OBOT_DISCOVERY_TEST_CA")
		pc.NetworkConfig.CACertPEM.Val = "resolved-CA-A"
		pc.CheckAndSetDefaults()
		return llmcoreclient.Config{
			Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Azure: pc},
			Keys:      map[schemas.ModelProvider][]schemas.Key{schemas.Azure: {key}},
			AzureAccounts: map[schemas.ModelProvider]map[string]llmcoreclient.AzureAccount{schemas.Azure: {
				"key": {SubscriptionID: "subscription", ResourceGroup: "group", AccountName: "account"},
			}},
			AzureDiscoveries: map[schemas.ModelProvider]map[string]llmcoreclient.AzureDiscoverySnapshot{schemas.Azure: {
				"key": {Names: []string{"deployment"}, BaseModels: map[string]string{"deployment": "base"}},
			}},
			Settings: map[string]json.RawMessage{},
		}
	}
	for _, tc := range []struct {
		name   string
		change func(*llmcoreclient.Config)
		same   bool
	}{
		{
			name:   "identical inputs",
			change: func(*llmcoreclient.Config) {},
			same:   true,
		},
		{
			name: "provider endpoint",
			change: func(cfg *llmcoreclient.Config) {
				pc := cfg.Providers[schemas.Azure]
				pc.NetworkConfig.BaseURL = "https://other.example"
				cfg.Providers[schemas.Azure] = pc
			},
		},
		{
			name: "resolved provider CA with same reference",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Providers[schemas.Azure].NetworkConfig.CACertPEM.Val = "resolved-CA-B"
			},
		},
		{
			name:   "key credential",
			change: func(cfg *llmcoreclient.Config) { cfg.Keys[schemas.Azure][0].Value.Val = "rotated" },
		},
		{
			name: "resolved credential with same reference",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Keys[schemas.Azure][0].AzureKeyConfig.ClientSecret.Val = "resolved-new-value"
			},
		},
		{
			name: "key endpoint",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Keys[schemas.Azure][0].AzureKeyConfig.Endpoint.Val = "https://other.openai.azure.com"
			},
		},
		{
			name: "aliases",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Keys[schemas.Azure][0].Aliases = schemas.KeyAliases{"alias": {ModelID: "deployment"}}
			},
		},
		{
			name:   "allowlist",
			change: func(cfg *llmcoreclient.Config) { cfg.Keys[schemas.Azure][0].Models = schemas.WhiteList{"deployment"} },
		},
		{
			name: "blocklist",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Keys[schemas.Azure][0].BlacklistedModels = schemas.BlackList{"deployment"}
			},
		},
		{
			name:   "disabled key",
			change: func(cfg *llmcoreclient.Config) { cfg.Keys[schemas.Azure][0].Enabled = new(false) },
		},
		{
			name:   "removed provider",
			change: func(cfg *llmcoreclient.Config) { delete(cfg.Providers, schemas.Azure) },
		},
		{
			name: "Azure account",
			change: func(cfg *llmcoreclient.Config) {
				cfg.AzureAccounts[schemas.Azure]["key"] = llmcoreclient.AzureAccount{SubscriptionID: "other"}
			},
		},
		{
			name: "Azure deployment names",
			change: func(cfg *llmcoreclient.Config) {
				cfg.AzureDiscoveries[schemas.Azure]["key"] = llmcoreclient.AzureDiscoverySnapshot{Names: []string{"other"}}
			},
		},
		{
			name: "Azure base model",
			change: func(cfg *llmcoreclient.Config) {
				cfg.AzureDiscoveries[schemas.Azure]["key"].BaseModels["deployment"] = "other-base"
			},
		},
		{
			name: "Azure refresh identity",
			change: func(cfg *llmcoreclient.Config) {
				snap := cfg.AzureDiscoveries[schemas.Azure]["key"]
				snap.RefreshID = "new-refresh"
				cfg.AzureDiscoveries[schemas.Azure]["key"] = snap
			},
		},
		{
			name: "global inference proxy",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Settings["proxy"] = json.RawMessage(`{"enabled":true,"enable_for_inference":true,"url":"http://proxy.example"}`)
			},
		},
		{
			name: "unrelated provider keys accounts discoveries",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Providers[schemas.OpenAI] = schemas.ProviderConfig{}
				cfg.Keys[schemas.OpenAI] = []schemas.Key{{Value: *schemas.NewSecretVar("unrelated")}}
				cfg.AzureAccounts[schemas.OpenAI] = map[string]llmcoreclient.AzureAccount{}
				cfg.AzureDiscoveries[schemas.OpenAI] = map[string]llmcoreclient.AzureDiscoverySnapshot{}
			},
			same: true,
		},
		{
			name: "unrelated settings and mappings",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Settings["pricing-overrides"] = json.RawMessage(`[]`)
				cfg.Settings[migrationSetting] = json.RawMessage(`{"mappings":{"old":"new"}}`)
			},
			same: true,
		},
		{
			name: "routing changes",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Rules = map[string]tables.TableRoutingRule{"unrelated": {ID: "unrelated", Enabled: new(true)}}
			},
			same: true,
		},
		{
			name: "disabled global proxy changes",
			change: func(cfg *llmcoreclient.Config) {
				cfg.Settings["proxy"] = json.RawMessage(`{"enabled":false,"url":"http://proxy.example"}`)
			},
			same: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fixture()
			before := modelDiscoveryConfig(cfg, schemas.Azure)
			tc.change(&cfg)
			require.Equal(t, tc.same, before == modelDiscoveryConfig(cfg, schemas.Azure))
		})
	}
	for _, field := range []string{"url", "password", "no_proxy", "skip_tls_verify"} {
		t.Run("effective proxy/"+field, func(t *testing.T) {
			cfg := fixture()
			proxy := map[string]any{"enabled": true, "enable_for_inference": true, "url": "http://proxy.example"}
			cfg.Settings["proxy"] = mustJSON(t, proxy)
			before := modelDiscoveryConfig(cfg, schemas.Azure)
			switch field {
			case "skip_tls_verify":
				proxy[field] = true
			default:
				proxy[field] = "changed"
			}
			cfg.Settings["proxy"] = mustJSON(t, proxy)
			require.NotEqual(t, before, modelDiscoveryConfig(cfg, schemas.Azure))
		})
	}
	t.Run("provider override ignores unrelated global proxy", func(t *testing.T) {
		cfg := fixture()
		pc := cfg.Providers[schemas.Azure]
		pc.ProxyConfig = &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://override.example")}
		cfg.Providers[schemas.Azure] = pc
		before := modelDiscoveryConfig(cfg, schemas.Azure)
		cfg.Settings["proxy"] = json.RawMessage(`{"enabled":true,"enable_for_inference":true,"url":"http://global.example"}`)
		require.Equal(t, before, modelDiscoveryConfig(cfg, schemas.Azure))
	})
}

func TestModelDiscoveryDirectInputsAndEmptySuccess(t *testing.T) {
	for _, phase := range []string{"desired but unapplied", "delayed A response"} {
		t.Run(phase, func(t *testing.T) {
			e, _ := migrationHealthFixture(t, interceptor.Funcs{})
			var pause atomic.Pointer[listingPause]
			var empty atomic.Bool
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if p := pause.Swap(nil); p != nil {
					p.block(r.Context())
				}
				if empty.Load() {
					_, _ = w.Write([]byte(`{"data":[]}`))
				} else {
					_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
				}
			}))
			t.Cleanup(up.Close)
			require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.BaseURL = up.URL
				return nil
			}))
			_, err := e.Sync(t.Context())
			require.NoError(t, err)
			require.Nil(t, directDiscoveryListing(t.Context(), e))
			cfgA, err := e.config(t.Context())
			require.NoError(t, err)
			require.Contains(t, e.availability.discoveredModels(cfgA), "openai/openai")
			var done chan *schemas.BifrostError
			var response *listingPause
			if phase == "delayed A response" {
				response = newListingPause(t)
				pause.Store(response)
				done = make(chan *schemas.BifrostError, 1)
				go func() { done <- directDiscoveryListing(t.Context(), e) }()
				response.wait(t)
			}
			require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
				p.Keys[0].Value = *schemas.NewSecretVar("B")
				return nil
			}))
			if phase == "desired but unapplied" {
				require.Nil(t, directDiscoveryListing(t.Context(), e), "public listing can still use applied A")
			}
			_, err = e.Sync(t.Context())
			require.NoError(t, err)
			cfgB, err := e.config(t.Context())
			require.NoError(t, err)
			if response != nil {
				close(response.release)
				select {
				case err := <-done:
					require.Nil(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("listing did not finish")
				}
			}
			require.NotContains(t, e.availability.discoveredModels(cfgB), "openai/openai")
			// Exercise readers concurrently with repeated replacements of provider evidence.
			stop, readerDone := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(readerDone)
				for {
					select {
					case <-stop:
						return
					default:
						_ = e.availability.discoveredModels(cfgA)
						_ = e.availability.discoveredModels(cfgB)
					}
				}
			}()
			defer func() { close(stop); <-readerDone }()
			require.Nil(t, directDiscoveryListing(t.Context(), e))
			require.Contains(t, e.availability.discoveredModels(cfgB), "openai/openai", "fresh B listing recovers without restart")
			empty.Store(true)
			require.Nil(t, directDiscoveryListing(t.Context(), e))
			current := e.availability.discoveredModels(cfgB)
			require.NotNil(t, current)
			require.NotContains(t, current, "openai/openai", "successful empty response replaces earlier IDs")
			require.Contains(t, current, "anthropic/anthropic", "other providers keep their evidence")
			require.NotNil(t, e.availability.discoveredModels(llmcoreclient.Config{}))
			require.Empty(t, e.availability.discoveredModels(llmcoreclient.Config{}), "absent or unreadable providers fail closed")
		})
	}
}

func TestModelDiscoveryRuntimeReplacementBetweenHooksAndWorker(t *testing.T) {
	for _, keyless := range []bool{false, true} {
		t.Run(map[bool]string{false: "keyed", true: "keyless"}[keyless], func(t *testing.T) {
			barrier := &listingBarrier{}
			e, _ := migrationHealthFixture(t, interceptor.Funcs{}, barrier)
			serve := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`)) }
			var bCalls atomic.Int32
			a := httptest.NewServer(http.HandlerFunc(serve))
			b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				bCalls.Add(1)
				serve(w, r)
			}))
			t.Cleanup(a.Close)
			t.Cleanup(b.Close)
			set := func(url string) {
				t.Helper()
				require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
					p.Config.NetworkConfig.BaseURL = url
					if keyless {
						p.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI, IsKeyLess: true}
					}
					return nil
				}))
				_, err := e.Sync(t.Context())
				require.NoError(t, err)
			}
			set(a.URL)
			pre, post := newListingPause(t), newListingPause(t)
			barrier.pre.Store(pre)
			barrier.post.Store(post)
			done := make(chan *schemas.BifrostError, 1)
			go func() { done <- directDiscoveryListing(t.Context(), e) }()
			pre.wait(t)
			set(b.URL)
			close(pre.release)
			post.wait(t)
			require.EqualValues(t, 1, bCalls.Load(), "the paused producer was rerouted to B")
			set(a.URL)
			close(post.release)
			select {
			case err := <-done:
				require.Nil(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("listing did not finish")
			}
			cfg, err := e.config(t.Context())
			require.NoError(t, err)
			require.True(t, e.core.Load().ProviderApplied(schemas.OpenAI, cfg))
			require.NotContains(t, e.availability.discoveredModels(cfg), "openai/openai", "a replacement queue served B between A hook boundaries")
			require.Nil(t, directDiscoveryListing(t.Context(), e))
			require.Contains(t, e.availability.discoveredModels(cfg), "openai/openai")
		})
	}
}

func TestModelDiscoveryAzureSnapshotFreshness(t *testing.T) {
	e, ctx := startTestEngine(t)
	key := azureTestKey("key")
	provider := azureTestProvider(key)
	provider.AzureDiscoveries = map[string]llmcoreclient.AzureDiscoverySnapshot{
		key.ID: {
			Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, provider.AzureAccounts[key.ID]),
			Names:       []string{"A"},
		},
	}
	require.NoError(t, e.store.CreateProvider(ctx, "azure", provider))
	_, err := e.Sync(ctx)
	require.NoError(t, err)
	_, err = e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	cfg, err := e.config(ctx)
	require.NoError(t, err)
	require.Contains(t, e.availability.discoveredModels(cfg), "azure/A")
	require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
		snapshot := p.AzureDiscoveries[key.ID]
		snapshot.Names = []string{"B"}
		p.AzureDiscoveries[key.ID] = snapshot
		return nil
	}))
	// Authoritative desired deployment lists remain usable before runtime reconciliation.
	models, err := e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	require.Equal(t, []schemas.Model{{ID: "azure/B"}}, models)
	cfg, err = e.config(ctx)
	require.NoError(t, err)
	require.Empty(t, e.availability.discoveredModels(cfg), "unapplied Azure snapshots cannot authorize cleanup")
	require.NoError(t, e.store.CreateProvider(ctx, "broken", llmcoreclient.Provider{}))
	_, err = e.Sync(ctx)
	require.ErrorContains(t, err, "broken")
	_, err = e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	require.Contains(t, e.availability.discoveredModels(cfg), "azure/B", "healthy Azure discovery advances despite unrelated errors")
	require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
		p.Keys[0].Enabled = new(false)
		return nil
	}))
	_, err = e.Sync(ctx)
	require.ErrorContains(t, err, "broken")
	cfg, err = e.config(ctx)
	require.NoError(t, err)
	require.Empty(t, e.availability.discoveredModels(cfg), "disabled keys cannot reuse previous evidence")
	_, err = e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	require.NotNil(t, e.availability.discoveredModels(cfg))
	require.Empty(t, e.availability.discoveredModels(cfg), "authoritative empty Azure listing clears discovery")
}
