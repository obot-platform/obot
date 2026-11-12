package llmengine

import (
	"context"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
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
	require.NoError(t, e.store.CreateProvider(ctx, "bedrock", llmcoreclient.Provider{Keys: []schemas.Key{
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
		require.Equal(t, "bedrock", LegacyProvider(scoped))
		key, err := e.LegacyKey(scoped, "bedrock/test")
		require.NoError(t, err)
		require.Equal(t, tc.want, key)
	}
	require.NoError(t, e.store.MutateProvider(ctx, "bedrock", func(p *llmcoreclient.Provider) error { p.Keys = nil; return nil }))
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	key, err := e.LegacyKey(WithLegacySource(ctx, system.AmazonBedrockModelProvider), "bedrock/test")
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
