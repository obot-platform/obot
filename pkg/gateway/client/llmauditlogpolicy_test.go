package client

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
	"uuid"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestLLMAuditBodyLimitPersistence(t *testing.T) {
	for _, limit := range []*int{nil, new(0), new(16)} {
		limitName := "unlimited"
		if limit != nil {
			limitName = fmt.Sprint(*limit)
		}

		for _, encrypted := range []bool{false, true} {
			t.Run(fmt.Sprintf("limit=%s/encrypted=%v", limitName, encrypted), func(t *testing.T) {
				c := newTestClient(t)
				WithLLMAuditLogBodyLimit(limit)(c)
				// Configuring LLM limits must not configure MCP limits.
				require.Nil(t, c.mcpAuditMaxBodyBytes)
				if encrypted {
					c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
						Transformers: map[schema.GroupResource]value.Transformer{
							llmAuditLogGroupResource: testTransformer{},
						},
					}
				}

				requestBody := json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`)
				modifiedBody := json.RawMessage(`{"messages":[{"role":"user","content":"modified"}]}`)
				responseBody := json.RawMessage(`{"id":"response-1","output":[{"text":"a long response"}]}`)
				for _, tt := range []struct {
					name       string
					provider   string
					path       string
					stream     []byte
					responseID string
				}{
					{
						name:       "direct",
						responseID: "response-1",
					},
					{
						name:       "queued JSON",
						responseID: "response-1",
						provider:   system.OpenAIModelProvider,
						path:       "/v1/responses",
						stream:     responseBody,
					},
					{
						name:     "OpenAI SSE",
						provider: system.OpenAIModelProvider,
						path:     "/v1/responses",
						stream:   []byte("data: {\"type\":\"response.completed\",\"response\":" + string(responseBody) + "}\n\ndata: [DONE]\n\n"),
					},
					{
						name:     "Anthropic SSE",
						provider: system.AnthropicModelProvider,
						path:     "/v1/messages",
						stream:   []byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"response-1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"a long response\"}}\n\n"),
					},
				} {
					t.Run(tt.name, func(t *testing.T) {
						entry := types.LLMAuditLog{
							ID:                        uuid.New().String(),
							ResponseID:                tt.responseID,
							CreatedAt:                 time.Now().UTC(),
							UserID:                    "user-1",
							ModelProvider:             tt.provider,
							RequestPath:               tt.path,
							RequestBody:               requestBody,
							PolicyModifiedRequestBody: modifiedBody,
							MessagePolicyTriggered:    true,
							ResponseBody:              responseBody,
							ResponseStatus:            200,
							Outcome:                   types.LLMAuditOutcomeSuccess,
							InputTokens:               42,
							OutputTokens:              17,
							RequestHeaders:            json.RawMessage(`{"header":"request"}`),
							ResponseHeaders:           json.RawMessage(`{"header":"response"}`),
						}
						original := entry
						expected := entry
						aggregateLLMAuditResponse(&expected, tt.stream)

						if tt.stream == nil {
							require.NoError(t, c.InsertLLMAuditLog(t.Context(), &entry))
						} else {
							c.LogLLMAuditEntry(entry, tt.stream)
							require.NoError(t, c.persistQueuedLLMAuditLogs())
						}
						require.Equal(t, original, entry, "must not modify caller-owned data")

						stored, err := c.GetLLMAuditLog(t.Context(), entry.ID, true)
						require.NoError(t, err)
						require.Equal(t, limitAuditBody(requestBody, limit), stored.RequestBody)
						require.Equal(t, limitAuditBody(modifiedBody, limit), stored.PolicyModifiedRequestBody)
						require.Equal(t, limitAuditBody(expected.ResponseBody, limit), stored.ResponseBody)
						require.Equal(t, entry.RequestHeaders, stored.RequestHeaders)
						require.Equal(t, entry.ResponseHeaders, stored.ResponseHeaders)
						require.Equal(t, entry.InputTokens, stored.InputTokens)
						require.Equal(t, entry.OutputTokens, stored.OutputTokens)
						require.Equal(t, entry.Outcome, stored.Outcome)
						require.Equal(t, entry.ResponseStatus, stored.ResponseStatus)
						require.True(t, stored.MessagePolicyTriggered)
						if tt.stream != nil {
							require.Equal(t, "response-1", stored.ResponseID)
						}

						// The API and exports use this conversion; previews must remain JSON.
						_, err = json.Marshal(types.ConvertLLMAuditLog(*stored))
						require.NoError(t, err)

						metadataOnly, err := c.GetLLMAuditLog(t.Context(), entry.ID, false)
						require.NoError(t, err)
						require.Empty(t, metadataOnly.RequestBody)
						require.Empty(t, metadataOnly.PolicyModifiedRequestBody)
						require.Empty(t, metadataOnly.ResponseBody)
					})
				}
			})
		}
	}
}
