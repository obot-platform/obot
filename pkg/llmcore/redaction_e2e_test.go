//go:build e2e

package llmcore_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/tests/e2e/harness"
	"github.com/stretchr/testify/require"
)

type redactionPolicies struct{ noPolicies }

func (redactionPolicies) ListGuardrails(context.Context) ([]types.Guardrail, error) {
	return []types.Guardrail{{ID: "secrets", Name: "secrets", Type: "secrets"}}, nil
}

func (redactionPolicies) ListGuardrailPolicies(context.Context) ([]types.GuardrailPolicy, error) {
	return []types.GuardrailPolicy{{
		ID:           "redact-secrets",
		Name:         "redact-secrets",
		Enabled:      true,
		Target:       types.GuardrailTargetLLM,
		ApplyTo:      types.GuardrailApplyOutput,
		Action:       types.GuardrailActionRedact,
		SamplingRate: 100,
		GuardrailIDs: []string{"secrets"},
		ProfileID:    "redaction-profile",
	}}, nil
}

func TestHTTPStreamRedactsSplitSecret(t *testing.T) {
	// The harmless AWS example key is split into two successive mock deltas.
	const secret = "AKIAIOSFODNN7EXAMPLE"
	mock, err := harness.StartMockLLM()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{
		Config: providerConfig(mock.URL),
		Keys:   []schemas.Key{apiKey("openai", openAIKey)},
	}))
	input, output := guardrails.New(redactionPolicies{}, nil).Plugins()
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{Plugins: []schemas.LLMPlugin{input, output}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })

	for _, api := range encodingAPIs {
		format, terminal := apiResponses, "event: response.completed\n"
		switch api.path {
		case "/v1/messages":
			format, terminal = apiMessages, "event: message_stop\n"
		case "/v1/chat/completions":
			format, terminal = apiChat, "data: [DONE]\n\n"
		}
		for _, protected := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/protected=%t", format, protected), func(t *testing.T) {
				identity := guardrails.Identity{}
				if protected {
					identity.ProfileIDs = []string{"redaction-profile"}
				}
				token, release := guardrails.BindRequest(identity, guardrails.RequestHooks{})
				defer release()
				body := strings.Replace(fmt.Sprintf(api.body, true), "hello", harness.Reply(secret)+" @@chunk:10@@", 1)
				req := httptest.NewRequest(http.MethodPost, api.path, strings.NewReader(body))
				req.Header.Set(guardrails.HeaderRequestToken, token)
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, req)

				result := parseLLM(format, response.Code, response.Header(), response.Body.Bytes()).requireOK(t)
				require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
				require.Contains(t, result.Body, terminal, "checked streams must finish normally")
				if protected {
					// Check the wire body too: later redaction cannot undo an earlier leak.
					require.NotContains(t, result.Body, secret[:10])
					require.NotContains(t, result.Body, secret[10:])
					require.Equal(t, "[REDACTED:AWS_ACCESS_KEY_ID]", result.Text)
				} else {
					// The control proves the fixture emits both fragments without the policy.
					require.Contains(t, result.Body, secret[:10])
					require.Contains(t, result.Body, secret[10:])
					require.Equal(t, secret, result.Text)
				}
			})
		}
	}
}
