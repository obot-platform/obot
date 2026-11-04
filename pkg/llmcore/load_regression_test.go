package llmcore_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	t.Cleanup(refreshed.Shutdown)
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
	t.Cleanup(degraded.Shutdown)
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
	t.Cleanup(engine.Shutdown)
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
