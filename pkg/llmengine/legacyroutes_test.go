package llmengine

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestLegacyCredentialFamilies(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "bedrock", llmcoreclient.Provider{Keys: []schemas.Key{
		{ID: "iam", Name: "imported-" + system.AmazonBedrockModelProvider},
		{ID: "api", Name: "imported-" + system.AmazonBedrockAPIKeyModelProvider},
	}}))
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
		key, err := e.LegacyKey(scoped, "bedrock")
		require.NoError(t, err)
		require.Equal(t, tc.want, key)
	}
	require.NoError(t, e.store.MutateProvider(ctx, "bedrock", func(p *llmcoreclient.Provider) error { p.Keys = nil; return nil }))
	key, err := e.LegacyKey(WithLegacySource(ctx, system.AmazonBedrockModelProvider), "bedrock")
	require.NoError(t, err)
	require.Empty(t, key)
}
