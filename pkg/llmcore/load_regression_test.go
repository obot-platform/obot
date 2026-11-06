package llmcore_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProviderReadErrorsQuarantineAndRecover(t *testing.T) {
	ctx := t.Context()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(upstream.Close)
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	for _, name := range []string{"healthy", "quarantined"} {
		require.NoError(t, store.CreateProvider(ctx, name, llmcoreclient.Provider{
			Config: schemas.ProviderConfig{
				NetworkConfig:        schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true},
				CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
			},
			Keys: []schemas.Key{{ID: "key", Name: "key", Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
		}))
	}
	refreshed, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, refreshed.Shutdown(context.Background())) })
	var providers v1.LLMGatewayProviderList
	require.NoError(t, storage.List(ctx, &providers))
	var corrupt *v1.LLMGatewayProvider
	for i := range providers.Items {
		if providers.Items[i].Spec.ID == "quarantined" {
			corrupt = &providers.Items[i]
		}
	}
	require.NotNil(t, corrupt)
	original := corrupt.Spec.Payload
	corrupt.Spec.Payload = "unreadable JSON"
	require.NoError(t, storage.Update(ctx, corrupt))
	degraded, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err, "unreadable provider must not block healthy startup")
	t.Cleanup(func() { require.NoError(t, degraded.Shutdown(context.Background())) })
	require.ErrorContains(t, degraded.Err(), "quarantined")
	for _, engine := range []*llmcore.Engine{refreshed, degraded} {
		for range 2 {
			_, err := engine.Sync(ctx)
			require.ErrorContains(t, err, "quarantined", "diagnostics must remain retryable without a new write")
			configured, err := engine.Client().GetConfiguredProviders()
			require.NoError(t, err)
			require.Contains(t, configured, schemas.ModelProvider("healthy"))
			require.NotContains(t, configured, schemas.ModelProvider("quarantined"))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"healthy/test","messages":[{"role":"user","content":"hello"}]}`)))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"quarantined/test","messages":[{"role":"user","content":"hello"}]}`)))
			require.GreaterOrEqual(t, response.Code, http.StatusBadRequest, "quarantined provider must stop serving")
		}
	}
	corrupt.Spec.Payload = original
	require.NoError(t, storage.Update(ctx, corrupt))
	for _, engine := range []*llmcore.Engine{refreshed, degraded} {
		_, err := engine.Sync(ctx)
		require.NoError(t, err)
		require.NoError(t, engine.Err())
		configured, err := engine.Client().GetConfiguredProviders()
		require.NoError(t, err)
		require.Contains(t, configured, schemas.ModelProvider("quarantined"))
	}
}

func TestBlockingLoadFailurePreservesRuntime(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(ctx, "openai", llmcoreclient.Provider{}))
	engine, err := llmcore.New(ctx, store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	invalid := &v1.LLMGatewayConfig{Name: system.LLMGatewayConfigName, Namespace: system.DefaultNamespace, Spec: v1.LLMGatewayResourceSpec{Payload: "invalid JSON"}}
	require.NoError(t, storage.Create(ctx, invalid))
	changed, err := engine.Sync(ctx)
	require.Error(t, err)
	require.False(t, changed)
	require.Error(t, engine.Err())
	configured, err := engine.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, configured, schemas.OpenAI, "a blocking load must not remove the running configuration")
	failed, err := llmcore.New(ctx, store, llmcore.Options{})
	require.Error(t, err)
	require.Nil(t, failed)
	require.NoError(t, storage.Delete(ctx, invalid))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.Err())
}

func TestStartupProviderFailuresRemainPending(t *testing.T) {
	ctx := t.Context()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(upstream.Close)
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(ctx, "healthy", transitionProvider(upstream.URL, "secret")))
	require.NoError(t, store.CreateProvider(ctx, "broken", llmcoreclient.Provider{}))
	log := &providerUpdateLogger{Logger: bifrost.NewDefaultLogger(schemas.LogLevelWarn), attempts: map[string]int{}}
	engine, err := llmcore.New(ctx, store, llmcore.Options{Logger: log})
	require.NoError(t, err, "one failed runtime must not block healthy startup")
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.ErrorContains(t, engine.Err(), "broken", "startup failures must be observable before Sync")
	healthy := engine.Client().GetProviderByKey("healthy")
	require.NotNil(t, healthy)
	require.Nil(t, engine.Client().GetProviderByKey("broken"))
	requestCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, attemptErr := engine.Client().ChatCompletionRequest(schemas.NewBifrostContext(requestCtx, schemas.NoDeadline), &schemas.BifrostChatRequest{
		Provider: "broken",
		Model:    "test",
		Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
	})
	require.NotNil(t, attemptErr)
	require.Contains(t, attemptErr.Error.Message, "being reconfigured", "startup failure must fail closed rather than wait on a workerless queue")

	for range 2 {
		changed, err := engine.Sync(ctx)
		require.True(t, changed)
		require.ErrorContains(t, err, "broken")
		require.ErrorContains(t, engine.Err(), "broken")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"healthy/test","messages":[{"role":"user","content":"hello"}]}`)))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	require.Equal(t, 2, log.count("broken"), "unchanged desired state must retry the failed runtime")
	require.Zero(t, log.count("healthy"), "healthy startup must not be restarted on retry")
	require.Same(t, healthy, engine.Client().GetProviderByKey("healthy"))

	require.NoError(t, store.MutateProvider(ctx, "broken", func(provider *llmcoreclient.Provider) error {
		*provider = transitionProvider(upstream.URL, "secret")
		return nil
	}))
	changed, err := engine.Sync(ctx)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, engine.Err())
	require.NotNil(t, engine.Client().GetProviderByKey("broken"))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"broken/test","messages":[{"role":"user","content":"hello"}]}`)))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	changed, err = engine.Sync(ctx)
	require.NoError(t, err)
	require.False(t, changed, "successful repair must converge")
	require.Same(t, healthy, engine.Client().GetProviderByKey("healthy"))
}
