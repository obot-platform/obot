package llmcore

import (
	"net/http/httptest"
	"testing"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	"github.com/stretchr/testify/require"
)

func TestInitialKeyRouting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		key      string
		want     string
	}{
		{
			name: "no rule",
			want: "initial",
		},
		{
			name:     "same provider",
			provider: "openai",
			want:     "initial",
		},
		{
			name:     "other provider",
			provider: "anthropic",
		},
		{
			name:     "rule key",
			provider: "anthropic",
			key:      "replacement",
			want:     "replacement",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
			store, err := rules.NewLocalStore(t.Context(), logger, nil)
			require.NoError(t, err)
			if tc.provider != "" {
				rule := policyRule("routed")
				rule.Targets = []tables.TableRoutingTarget{{Provider: &tc.provider, KeyID: &tc.key, Weight: 1}}
				require.NoError(t, store.UpsertRule(t.Context(), &rule))
			}
			p, err := newRoutingPlugin(store, logger, nil)
			require.NoError(t, err)
			e := &Engine{account: &account{}}
			req := httptest.NewRequest("POST", "/v1/responses", nil).WithContext(WithInitialKey(t.Context(), "initial"))
			ctx, cancel := e.requestContext(req)
			defer cancel()
			require.NoError(t, p.PreRequestHook(ctx, policyRequest()))
			require.Equal(t, tc.want, ctx.Value(schemas.BifrostContextKeyRoutingPinnedAPIKeyID))
			nested, done := e.inferenceContext(ctx)
			defer done()
			require.Empty(t, nested.Value(initialKeyContextKey{}))
		})
	}
}
