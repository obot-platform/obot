package llmengine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestLegacyCredentialFamilies(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "bedrock_mantle", llmcoreclient.Provider{Keys: []schemas.Key{
		{ID: "iam", Name: "imported-" + system.AmazonBedrockModelProvider, Models: schemas.WhiteList{"*"}},
		{ID: "api", Name: "imported-" + system.AmazonBedrockAPIKeyModelProvider, Models: schemas.WhiteList{"*"}},
	}}))
	_, err := e.Sync(ctx)
	require.NoError(t, err)
	for _, tc := range []struct {
		source string
		want   string
	}{
		{
			source: system.AmazonBedrockModelProvider,
			want:   "iam",
		},
		{
			source: system.AmazonBedrockAPIKeyModelProvider,
			want:   "api",
		},
	} {
		scoped := WithLegacySource(ctx, tc.source)
		require.Equal(t, "bedrock_mantle", LegacyProvider(scoped))
		key, err := e.LegacyKey(scoped, "bedrock_mantle/test")
		require.NoError(t, err)
		require.Equal(t, tc.want, key)
	}
	require.NoError(t, e.store.MutateProvider(ctx, "bedrock_mantle", func(p *llmcoreclient.Provider) error { p.Keys = nil; return nil }))
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	key, err := e.LegacyKey(WithLegacySource(ctx, system.AmazonBedrockModelProvider), "bedrock_mantle/test")
	require.NoError(t, err)
	require.Empty(t, key)
}

func TestLegacyKeyReadsOnlyReconciledConfiguration(t *testing.T) {
	reads := 0
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			reads++
			return c.Get(ctx, key, obj, opts...)
		},
		List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			reads++
			return c.List(ctx, list, opts...)
		},
	}).Build()
	store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{Keys: []schemas.Key{
		{ID: "original", Name: "imported-" + system.OpenAIModelProvider, Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1},
	}}))
	e := New()
	require.NoError(t, e.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	ctx := WithLegacySource(t.Context(), system.OpenAIModelProvider)
	check := func(want string) {
		t.Helper()
		reads = 0
		key, err := e.LegacyKey(ctx, "openai/test")
		require.NoError(t, err)
		require.Equal(t, want, key)
		require.Zero(t, reads, "request-time key lookup must not read configuration storage")
	}
	check("original")
	require.NoError(t, store.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
		p.Keys[0].ID = "rotated"
		return nil
	}))
	check("original")
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	check("rotated")
	require.NoError(t, store.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
		p.Keys[0].Name = "manually-configured"
		return nil
	}))
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	check("")
	_, err = e.LegacyKey(WithLegacySource(ctx, system.AnthropicModelProvider), "anthropic/test")
	require.True(t, apierrors.IsNotFound(err))
	_, err = New().LegacyKey(ctx, "openai/test")
	require.True(t, apierrors.IsServiceUnavailable(err))
}

func TestLegacyKeyDuringProviderReconfiguration(t *testing.T) {
	upstream := mockOpenAI(t)
	t.Cleanup(upstream.Close)
	e, ctx := startTestEngine(t)
	for _, provider := range []string{"generic-responses", "healthy"} {
		require.NoError(t, e.store.CreateProvider(ctx, provider, llmcoreclient.Provider{
			Config: schemas.ProviderConfig{
				NetworkConfig:        schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true},
				CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
			},
			Keys: []schemas.Key{{ID: provider, Name: "imported-" + system.GenericResponsesModelProvider, Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
		}))
	}
	_, err := e.Sync(ctx)
	require.NoError(t, err)
	ctx = WithLegacySource(ctx, system.GenericResponsesModelProvider)
	key, err := e.LegacyKey(ctx, "generic-responses/test")
	require.NoError(t, err)
	require.Equal(t, "generic-responses", key)

	// A failed replacement keeps the provider transitioning without timing-dependent pauses.
	require.NoError(t, e.store.MutateProvider(ctx, "generic-responses", func(p *llmcoreclient.Provider) error {
		p.Config.CustomProviderConfig.BaseProviderType = "unsupported-base"
		return nil
	}))
	_, err = e.Sync(ctx)
	require.Error(t, err)
	_, _, err = e.core.Load().ProviderKeyID(ctx, "generic-responses", "imported-"+system.GenericResponsesModelProvider, "test")
	require.ErrorContains(t, err, "being reconfigured")
	key, err = e.LegacyKey(ctx, "generic-responses/test")
	require.NoError(t, err)
	require.Empty(t, key)

	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{
			name: "provider gate",
			body: `{"model":"generic-responses/test","messages":[{"role":"user","content":"hello"}]}`,
			code: http.StatusServiceUnavailable,
		},
		{
			name: "fallback",
			body: `{"model":"generic-responses/test","fallbacks":["healthy/test"],"messages":[{"role":"user","content":"hello"}]}`,
			code: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body)).WithContext(llmcore.WithInitialKey(ctx, key))
			e.ServeHTTP(response, request)
			require.Equal(t, tc.code, response.Code, response.Body.String())
		})
	}
}
