package llmcore

import (
	"net/http"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
)

func TestDirectClientCredentialsAreRefusedOnEveryAttempt(t *testing.T) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(t.Context())
	defer cancel()
	plugin := &credentialPlugin{}
	req := &schemas.BifrostRequest{RequestType: schemas.ChatCompletionRequest, ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.Anthropic, Model: "test"}}
	_, short, err := plugin.PreLLMHook(ctx, req)
	require.NoError(t, err)
	require.Nil(t, short)
	// Reusing a context cannot turn an earlier credential-free attempt into an exemption.
	for _, headers := range []map[string][]string{
		{"Authorization": {"Bearer sk-ant-oat-fixture"}},
		{"authorization": {"Bearer unrelated-fixture"}},
		{"X-Api-Key": {"caller-fixture"}},
	} {
		ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, headers)
		_, short, err = plugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, short)
		require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
		require.False(t, *short.Error.AllowFallbacks)
	}
}
