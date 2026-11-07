package llmengine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestProviderQueueDefaults(t *testing.T) {
	e, ctx := startTestEngine(t)
	var created schemas.ProviderConfig
	require.NoError(t, testMutation(ctx, e, &created)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai"}))))
	require.Positive(t, created.ConcurrencyAndBufferSize.Concurrency, "API must show the effective worker count")
	require.Positive(t, created.ConcurrencyAndBufferSize.BufferSize)
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, created.ConcurrencyAndBufferSize, cfg.Config.Providers[schemas.OpenAI].ConcurrencyAndBufferSize)
	var updated schemas.ProviderConfig
	require.NoError(t, testMutation(ctx, e, &updated)(e.UpdateProvider(ctx, "openai", testJSON(t, map[string]any{"concurrency_and_buffer_size": map[string]any{"concurrency": 0, "buffer_size": 0}}), false)))
	require.Positive(t, updated.ConcurrencyAndBufferSize.Concurrency)
	require.Positive(t, updated.ConcurrencyAndBufferSize.BufferSize)
	require.Error(t, testMutation(ctx, e, nil)(e.UpdateProvider(ctx, "openai", testJSON(t, map[string]any{"concurrency_and_buffer_size": map[string]any{"concurrency": 0, "buffer_size": 1}}), false)), "validate effective defaults against the requested queue size")
}

func TestOmittedQueueStillServesInference(t *testing.T) {
	e, ctx := startTestEngine(t)
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "test-secret", "models": []string{"*"}, "weight": 1}))))
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	require.NoError(t, testInference(bounded, t, e, http.MethodPost, "/v1/chat/completions", map[string]any{"model": "openai/gpt-4o-mini", "messages": []map[string]any{{"role": "user", "content": "hello"}}}, nil))
}

func TestConcurrentEnvImport(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "env-openai-secret")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	base, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	var arrivals atomic.Int32
	ready := make(chan struct{})
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok {
				if arrivals.Add(1) == 2 {
					close(ready)
				}
				select {
				case <-ready:
				case <-ctx.Done():
					return ctx.Err()
				}
				err := c.Create(ctx, obj, opts...)
				if err == nil {
					return base.PutKey(ctx, "openai", schemas.Key{ID: "manual", Name: "manual", Value: *schemas.NewSecretVar("manual-secret")})
				}
				return err
			}
			return c.Create(ctx, obj, opts...)
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	opts := Options{Store: store, DataDir: t.TempDir()}
	a, b := New(), New()
	for _, e := range []*Engine{a, b} {
		require.NoError(t, e.Start(ctx, opts))
		t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	}
	mp := ModelProviderResource()
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
	results := make(chan bool, 2)
	go func() { results <- a.importEnvProviders(ctx, c, mp.DeepCopy()) }()
	go func() { results <- b.importEnvProviders(ctx, c, mp.DeepCopy()) }()
	for range 2 {
		select {
		case imported := <-results:
			require.True(t, imported, "each replica must finish the import after a concurrent create")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	cfg, err := a.store.Load(ctx)
	require.NoError(t, err)
	require.Len(t, cfg.Config.Keys[schemas.OpenAI], 2)
	values := map[string]string{}
	for _, key := range cfg.Config.Keys[schemas.OpenAI] {
		values[key.Name] = key.Value.GetValue()
	}
	require.Equal(t, "env-openai-secret", values["env-"+system.OpenAIAPIKeyEnvVar])
	require.Equal(t, "manual-secret", values["manual"], "concurrent imports must preserve an independently added key")
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(mp), mp))
	require.NotEmpty(t, mp.Annotations["obot.ai/llm-gateway-env-key-openai"])
}

func TestRefreshModelsFetchesUpstream(t *testing.T) {
	e, ctx := startTestEngine(t)
	var generation atomic.Int32
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if generation.Load() == 2 {
			http.Error(w, "listing unavailable", http.StatusInternalServerError)
			return
		}
		if generation.Load() == 3 {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			return
		}
		model := "gpt-old"
		if generation.Load() == 1 {
			model = "gpt-new"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": model, "object": "model"}}})
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "test-secret", "models": []string{"*"}, "weight": 1}))))
	before, err := e.store.Load(ctx)
	require.NoError(t, err)
	pending, err := e.RefreshProviderModels(ctx, "openai")
	require.NoError(t, err)
	require.False(t, pending)
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-old")
	generation.Store(1)
	pending, err = e.RefreshProviderModels(ctx, "openai")
	require.NoError(t, err)
	require.False(t, pending)
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new")
	require.NotContains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-old")
	after, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "model refresh is not a configuration write")
	require.EqualValues(t, 2, calls.Load())
	generation.Store(2)
	_, err = e.RefreshProviderModels(ctx, "openai")
	require.Error(t, err)
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new", "a failed refresh preserves the last successful listing")
	generation.Store(3)
	pending, err = e.RefreshProviderModels(ctx, "openai")
	require.NoError(t, err)
	require.False(t, pending)
	require.NotContains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new", "an empty successful listing replaces cached models")
}

func TestCanceledAfterCommitStillReturnsSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			err := c.Create(ctx, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && err == nil {
				cancel()
			}
			return err
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	require.NoError(t, e.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	_, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	cfg, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Contains(t, cfg.Config.Providers, schemas.OpenAI)
}

func TestSyncFailureAfterCommitStillReturnsSuccess(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.core.Load().Shutdown(context.Background()))
	out, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)
	require.Equal(t, schemas.OpenAI, out.Name)
	_, err = e.Sync(ctx)
	require.Error(t, err)
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, cfg.Config.Providers, schemas.OpenAI)
	require.Error(t, e.Err(), "synchronization failure remains observable through status")
}

func TestKeyRenamePreservesUniqueNames(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai"}))))
	var first, second schemas.Key
	require.NoError(t, testMutation(ctx, e, &first)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "first", "value": "secret-one"}))))
	require.NoError(t, testMutation(ctx, e, &second)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "second", "value": "secret-two"}))))
	id := first.ID
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "openai", id, testJSON(t, map[string]any{"name": "first", "id": second.ID}))), "keeping the same name is allowed")
	before, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Error(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "openai", id, testJSON(t, map[string]any{"name": second.Name}))))
	after, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "rejected rename must not commit")
	require.NoError(t, testMutation(ctx, e, nil)(e.UpdateKey(ctx, "openai", id, testJSON(t, map[string]any{"name": "renamed"}))))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	names := map[string]string{}
	for _, key := range cfg.Config.Keys[schemas.OpenAI] {
		names[key.ID] = key.Name
	}
	require.Equal(t, "renamed", names[first.ID])
	require.Equal(t, "second", names[second.ID])
}

func TestPublicWriteDoesNotWaitForRuntime(t *testing.T) {
	e, ctx := startTestEngine(t)
	e.syncMu.Lock()
	unlock := sync.OnceFunc(e.syncMu.Unlock)
	defer unlock()
	done := make(chan error, 1)
	go func() {
		_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("persisted write waited for runtime synchronization")
	}
	providers, err := e.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Empty(t, providers, "public write must leave application to reconciliation")
	unlock()
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	providers, err = e.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, providers, schemas.OpenAI)
}

func TestEnvImportRetriesResourceConflict(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "env-secret")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	ctx := t.Context()
	mp := ModelProviderResource()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
	base, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	require.NoError(t, base.CreateProvider(ctx, "openai", llmcoreclient.Provider{}))
	injected := false
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, options ...kclient.UpdateOption) error {
			if _, provider := obj.(*v1.LLMGatewayProvider); provider && !injected {
				injected = true
				require.NoError(t, base.PutKey(ctx, "openai", schemas.Key{ID: "manual", Name: "manual", Value: *schemas.NewSecretVar("manual-secret")}))
				return apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayproviders"}, obj.GetName(), nil)
			}
			return client.Update(ctx, obj, options...)
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	engine := New()
	require.NoError(t, engine.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.True(t, engine.importEnvProviders(ctx, storage, mp))
	require.True(t, injected)
	provider, err := base.GetProvider(ctx, "openai")
	require.NoError(t, err)
	values := map[string]string{}
	for _, key := range provider.Keys {
		values[key.Name] = key.Value.GetValue()
	}
	require.Equal(t, "manual-secret", values["manual"])
	require.Equal(t, "env-secret", values["env-"+system.OpenAIAPIKeyEnvVar])
	require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(mp), mp))
	require.NotEmpty(t, mp.Annotations["obot.ai/llm-gateway-env-key-openai"])
}

func TestDirectProviderGuardsUseOwningSnapshot(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	blockLists := false
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if blockLists {
				return fmt.Errorf("unexpected aggregate read during scoped mutation")
			}
			return c.List(ctx, list, opts...)
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	require.NoError(t, e.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	ctx := t.Context()
	_, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"vllm","network_config":{"allow_private_network":true}}`))
	require.NoError(t, err)
	blockLists = true

	_, err = e.CreateKey(ctx, "vllm", json.RawMessage(`{"name":"metadata","value":"secret","vllm_key_config":{"url":"http://169.254.169.254"}}`))
	require.ErrorContains(t, err, "can't be used")
	key, err := e.CreateKey(ctx, "vllm", json.RawMessage(`{"name":"primary","value":"secret-with-enough-characters","vllm_key_config":{"url":"http://10.1.2.3:8000"}}`))
	require.NoError(t, err)
	patch := object(key)
	patch["vllm_key_config"] = map[string]any{"url": "http://10.1.2.4:8000"}
	_, err = e.UpdateKey(ctx, "vllm", key.ID, testJSON(t, patch))
	require.ErrorContains(t, err, "entering its secrets again")
	before, err := store.GetProvider(ctx, "vllm")
	require.NoError(t, err)

	patch = map[string]any{"network_config": map[string]any{"base_url": "http://127.0.0.1:9000", "allow_private_network": true}}
	_, err = e.UpdateProvider(ctx, "vllm", testJSON(t, patch), false)
	require.ErrorContains(t, err, "removes its keys")
	patch["concurrency_and_buffer_size"] = map[string]any{"concurrency": 2, "buffer_size": 1}
	_, err = e.UpdateProvider(ctx, "vllm", testJSON(t, patch), true)
	require.ErrorContains(t, err, "Concurrency must be less")
	after, err := store.GetProvider(ctx, "vllm")
	require.NoError(t, err)
	require.Equal(t, before, after, "failed validation must roll back confirmed key removal too")

	delete(patch, "concurrency_and_buffer_size")
	_, err = e.UpdateProvider(ctx, "vllm", testJSON(t, patch), true)
	require.NoError(t, err)
	after, err = store.GetProvider(ctx, "vllm")
	require.NoError(t, err)
	require.Empty(t, after.Keys)
	require.Equal(t, "http://127.0.0.1:9000", after.Config.NetworkConfig.BaseURL)
}
