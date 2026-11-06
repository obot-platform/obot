package guardrails

import (
	"context"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llm"
	"github.com/stretchr/testify/require"
)

func TestTrustedSubscriptionAttemptClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider schemas.ModelProvider
		agent    string
		skip     bool
		direct   bool
		marker   *bool
		want     bool
	}{
		{
			name:     "legacy Claude mode unchanged",
			provider: schemas.Anthropic,
			agent:    "claude-cli/2",
			skip:     true,
			want:     true,
		},
		{
			name:     "ordinary Codex with direct key is not inferred",
			provider: schemas.OpenAI,
			agent:    "codex_cli_rs/1",
			direct:   true,
		},
		{
			name:     "ordinary Codex with configured key",
			provider: schemas.OpenAI,
			agent:    "codex-cli/1",
		},
		{
			name:     "trusted engine admitted Claude",
			provider: schemas.Anthropic,
			agent:    "claude-cli/2",
			skip:     true,
			marker:   new(true),
			want:     true,
		},
		{
			name:     "trusted refusal overrides legacy inference",
			provider: schemas.Anthropic,
			agent:    "claude-cli/2",
			skip:     true,
			marker:   new(false),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
			defer ctx.Cancel()
			ctx.SetValue(schemas.BifrostContextKeyUserAgent, tc.agent)
			ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, tc.skip)
			if tc.direct {
				ctx.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: "ordinary-direct"})
			}
			if tc.marker != nil {
				llm.SetSubscriptionAttempt(ctx, *tc.marker)
			}
			var attempts []Attempt
			token, release := BindRequest(Identity{}, RequestHooks{AttemptCheck: func(_ context.Context, attempt Attempt) error {
				attempts = append(attempts, attempt)
				return nil
			}})
			defer release()
			ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{HeaderRequestToken: token})
			req := &schemas.BifrostRequest{ResponsesRequest: &schemas.BifrostResponsesRequest{Provider: tc.provider, Model: "fixture"}}
			require.Nil(t, checkAttempt(ctx, req))
			require.Equal(t, []Attempt{{Model: string(tc.provider) + "/fixture", Subscription: tc.want}}, attempts)
		})
	}
}
