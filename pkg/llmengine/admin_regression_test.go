package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type importBarrierTransport struct {
	next    http.RoundTripper
	arrived *atomic.Int32
	ready   chan struct{}
}

func (t importBarrierTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(req)
	if err == nil && req.Method == http.MethodGet && req.URL.Path == "/api/providers/openai/keys" && resp.StatusCode == http.StatusNotFound {
		if t.arrived.Add(1) == 2 {
			close(t.ready)
		}
		select {
		case <-t.ready:
		case <-req.Context().Done():
			_ = resp.Body.Close()
			return nil, req.Context().Err()
		}
	}
	return resp, err
}

func TestProviderQueueDefaults(t *testing.T) {
	e, ctx := startTestEngine(t)
	var created schemas.ProviderConfig
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, &created))
	require.Positive(t, created.ConcurrencyAndBufferSize.Concurrency, "API must show the effective worker count")
	require.Positive(t, created.ConcurrencyAndBufferSize.BufferSize)
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, created.ConcurrencyAndBufferSize, cfg.Providers[schemas.OpenAI].ConcurrencyAndBufferSize)
	var updated schemas.ProviderConfig
	require.NoError(t, e.Do(ctx, http.MethodPut, "/api/providers/openai", map[string]any{"concurrency_and_buffer_size": map[string]any{"concurrency": 0, "buffer_size": 0}}, &updated))
	require.Positive(t, updated.ConcurrencyAndBufferSize.Concurrency)
	require.Positive(t, updated.ConcurrencyAndBufferSize.BufferSize)
	require.Error(t, e.Do(ctx, http.MethodPut, "/api/providers/openai", map[string]any{"concurrency_and_buffer_size": map[string]any{"concurrency": 0, "buffer_size": 1}}, nil), "validate effective defaults against the requested queue size")
}

func TestOmittedQueueStillServesInference(t *testing.T) {
	e, ctx := startTestEngine(t)
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}, nil))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "test", "value": "test-secret", "models": []string{"*"}, "weight": 1}, nil))
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	require.NoError(t, e.Do(bounded, http.MethodPost, "/v1/chat/completions", map[string]any{"model": "openai/gpt-4o-mini", "messages": []map[string]any{{"role": "user", "content": "hello"}}}, nil))
}

func TestConcurrentEnvImport(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "env-openai-secret")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	opts := testOptions(t, Options{DataDir: t.TempDir()})
	a, b := New(), New()
	for _, e := range []*Engine{a, b} {
		require.NoError(t, e.Start(ctx, opts))
		t.Cleanup(e.Shutdown)
	}
	var arrivals atomic.Int32
	ready := make(chan struct{})
	for _, e := range []*Engine{a, b} {
		e.client.Transport = importBarrierTransport{next: e.client.Transport, arrived: &arrivals, ready: ready}
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
	require.Len(t, cfg.Keys[schemas.OpenAI], 1)
	require.Equal(t, "env-openai-secret", cfg.Keys[schemas.OpenAI][0].Value.GetValue())
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
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}, nil))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "test", "value": "test-secret", "models": []string{"*"}, "weight": 1}, nil))
	before, err := e.store.Revision(ctx)
	require.NoError(t, err)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/refresh-models", map[string]any{}, nil))
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-old")
	generation.Store(1)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/refresh-models", map[string]any{}, nil))
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new")
	require.NotContains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-old")
	after, err := e.store.Revision(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "model refresh is not a configuration write")
	require.EqualValues(t, 2, calls.Load())
	generation.Store(2)
	require.Error(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/refresh-models", map[string]any{}, nil))
	require.Contains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new", "a failed refresh preserves the last successful listing")
	generation.Store(3)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/refresh-models", map[string]any{}, nil))
	require.NotContains(t, e.prices.GetUnfilteredModelsForProvider(schemas.OpenAI), "gpt-new", "an empty successful listing replaces cached models")
}

func TestCanceledAfterCommitStillReturnsSuccess(t *testing.T) {
	e, _ := startTestEngine(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/providers", strings.NewReader(`{"provider":"openai"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	e.syncMu.Lock()
	unlock := sync.OnceFunc(e.syncMu.Unlock)
	defer unlock()
	done := make(chan struct{})
	go func() { defer close(done); e.ServeHTTP(rec, req) }()
	require.Eventually(t, func() bool { rev, err := e.store.Revision(t.Context()); return err == nil && rev > 0 }, 3*time.Second, time.Millisecond)
	cancel()
	unlock()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not finish")
	}
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	cfg, err := e.store.Load(t.Context())
	require.NoError(t, err)
	require.Contains(t, cfg.Providers, schemas.OpenAI)
}

func TestSyncFailureAfterCommitStillReturnsSuccess(t *testing.T) {
	e, ctx := startTestEngine(t)
	e.core.Load().Shutdown()
	var out map[string]any
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, &out))
	require.Equal(t, "openai", out["name"])
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	require.Contains(t, cfg.Providers, schemas.OpenAI)
	require.Error(t, e.Err(), "synchronization failure remains observable through status")
}

func TestKeyRenamePreservesUniqueNames(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil))
	var first, second schemas.Key
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "first", "value": "secret-one"}, &first))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "second", "value": "secret-two"}, &second))
	path := "/api/providers/openai/keys/" + first.ID
	require.NoError(t, e.Do(ctx, http.MethodPut, path, map[string]any{"name": "first"}, nil), "keeping the same name is allowed")
	before, err := e.store.Revision(ctx)
	require.NoError(t, err)
	require.Error(t, e.Do(ctx, http.MethodPut, path, map[string]any{"name": second.Name}, nil))
	after, err := e.store.Revision(ctx)
	require.NoError(t, err)
	require.Equal(t, before, after, "rejected rename must not commit")
	require.NoError(t, e.Do(ctx, http.MethodPut, path, map[string]any{"name": "renamed"}, nil))
	cfg, err := e.store.Load(ctx)
	require.NoError(t, err)
	names := map[string]string{}
	for _, key := range cfg.Keys[schemas.OpenAI] {
		names[key.ID] = key.Name
	}
	require.Equal(t, "renamed", names[first.ID])
	require.Equal(t, "second", names[second.ID])
}
