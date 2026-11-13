package llmengine

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestFreshAndImportedMantleServeClaudeAndGPT(t *testing.T) {
	for _, mode := range []string{"fresh", "imported"} {
		t.Run(mode, func(t *testing.T) {
			type attempt struct{ path, auth, model string }
			attempts := make(chan attempt, 20)
			upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, `{"data":[]}`)
					return
				}
				var body struct {
					Model string `json:"model"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				attempts <- attempt{
					path:  r.URL.Path,
					auth:  r.Header.Get("Authorization"),
					model: body.Model,
				}
				switch r.URL.Path {
				case "/anthropic/v1/messages":
					_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","model":"claude-haiku-4-5-20251001","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				case "/openai/v1/responses":
					_, _ = io.WriteString(w, `{"id":"resp_test","object":"response","status":"completed","model":"openai.gpt-5.4","output":[{"type":"message","id":"msg_test","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
				default:
					http.Error(w, "wrong AWS surface: "+r.URL.Path, http.StatusBadRequest)
				}
			}))
			t.Cleanup(upstream.Close)
			e, ctx := startTestEngine(t)
			if mode == "imported" {
				gateway := legacyImportGateway(t, map[string]map[string]string{
					system.AmazonBedrockModelProvider: {
						"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access",
						"OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret",
					},
					system.AmazonBedrockAPIKeyModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "api-token"},
				})
				require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, true))
			} else {
				_, err := e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "bedrock_mantle"}))
				require.NoError(t, err)
				for _, payload := range []map[string]any{
					{"name": "iam", "value": "", "models": []string{"*"}, "weight": 1, "bedrock_mantle_key_config": map[string]any{"region": "us-east-1", "access_key": "access", "secret_key": "secret"}},
					{"name": "api", "value": "api-token", "models": []string{"*"}, "weight": 1, "bedrock_mantle_key_config": map[string]any{"region": "us-east-1"}},
				} {
					_, err = e.CreateKey(ctx, "bedrock_mantle", testJSON(t, payload))
					require.NoError(t, err)
				}
			}
			// Exercise the real adapter with a local AWS endpoint, including IAM signing.
			require.NoError(t, e.store.MutateProvider(ctx, "bedrock_mantle", func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.InsecureSkipVerify = true
				p.Config.NetworkConfig.AllowPrivateNetwork = true
				for i := range p.Keys {
					p.Keys[i].BedrockMantleKeyConfig.Endpoints = &schemas.BedrockEndpoints{Mantle: schemas.NewSecretVar(strings.TrimPrefix(upstream.URL, "https://"))}
				}
				return nil
			}))
			_, err := e.Sync(ctx)
			require.NoError(t, err)
			provider, err := e.store.GetProvider(ctx, "bedrock_mantle")
			require.NoError(t, err)
			require.Len(t, provider.Keys, 2)
			for _, key := range provider.Keys {
				for _, model := range []string{"anthropic.claude-haiku-4-5", "openai.gpt-5.4"} {
					for _, protocol := range []string{"messages", "responses"} {
						body := fmt.Sprintf(`{"model":"bedrock_mantle/%s","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`, model)
						if protocol == "responses" {
							body = fmt.Sprintf(`{"model":"bedrock_mantle/%s","input":"hi","max_output_tokens":16}`, model)
						}
						req := httptest.NewRequest(http.MethodPost, "/v1/"+protocol, strings.NewReader(body)).WithContext(llmcore.WithInitialKey(ctx, key.ID))
						w := httptest.NewRecorder()
						e.ServeHTTP(w, req)
						require.Equal(t, http.StatusOK, w.Code, w.Body.String())
						a := <-attempts
						require.Equal(t, model, a.model)
						if strings.HasPrefix(model, "anthropic.") {
							require.Equal(t, "/anthropic/v1/messages", a.path)
						} else {
							require.Equal(t, "/openai/v1/responses", a.path)
						}
						if key.Value.GetValue() != "" {
							require.Equal(t, "Bearer api-token", a.auth)
						} else {
							require.Contains(t, a.auth, "AWS4-HMAC-SHA256 Credential=access/")
						}
					}
				}
			}
		})
	}
}
