package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/messagepolicy"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNativeJudgeDoesNotInheritOutputGuardrails(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var judgeCalls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model  string `json:"model"`
					Stream bool   `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				text := "original answer"
				if request.Model == "judge" {
					// Bound the regression: inherited output state must not recurse indefinitely.
					if judgeCalls.Add(1) > 1 {
						http.Error(w, `{"error":{"message":"recursive judge call"}}`, http.StatusBadRequest)
						return
					}
					text = "yes"
				}
				if request.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprintf(w, "data: {\"id\":\"outer\",\"object\":\"chat.completion.chunk\",\"model\":\"outer\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q,\"tool_calls\":[{\"index\":0,\"id\":\"call_outer\",\"type\":\"function\",\"function\":{\"name\":\"weather\",\"arguments\":\"{}\"}}]}}]}\n\n", text)
					_, _ = io.WriteString(w, "data: {\"id\":\"outer\",\"object\":\"chat.completion.chunk\",\"model\":\"outer\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"id":"reply","object":"chat.completion","model":%q,"choices":[{"index":0,"message":{"role":"assistant","content":%q,"tool_calls":[{"id":"call_test","type":"function","function":{"name":"weather","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, request.Model, text)
				}
			}))
			t.Cleanup(up.Close)

			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
			store, err := llmcoreclient.New(storage, plaintextGatewayCodec{})
			require.NoError(t, err)
			require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{
				Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{
					BaseURL:             up.URL,
					AllowPrivateNetwork: true,
				}},
				Keys: []schemas.Key{{
					ID:     "test-key",
					Name:   "test-key",
					Value:  *schemas.NewSecretVar("test-secret"),
					Models: schemas.WhiteList{"*"},
					Weight: 1,
				}},
			}))
			model := &v1.Model{
				Name:      "judge-model",
				Namespace: system.DefaultNamespace,
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					Active:        true,
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   "openai/judge",
				}},
			}
			require.NoError(t, storage.Create(t.Context(), model))
			require.NoError(t, storage.Create(t.Context(), &v1.Alias{
				Name: alias.KeyFromScopeID("Model", string(types.DefaultModelAliasTypeLLMMini)),
				Spec: v1.AliasSpec{
					TargetKind:      "Model",
					TargetNamespace: model.Namespace,
					TargetName:      model.Name,
				},
			}))
			gw := llmengine.New()
			outputPolicy := policy("judge-output", gatewaytypes.GuardrailActionBlock, "judge")
			outputPolicy.ApplyTo = gatewaytypes.GuardrailApplyOutput
			outputPolicy.FailClosed = true
			policyStore := &fakeStore{
				guardrails: []gatewaytypes.Guardrail{guardrail("judge", JudgeType, map[string]any{"definition": "Allow this answer"})},
				policies:   []gatewaytypes.GuardrailPolicy{outputPolicy},
			}
			guardrails := New(policyStore, messagepolicy.NewHelper(storage, gw))
			input, output := guardrails.Plugins()
			dir := t.TempDir()
			modeldatatest.Write(t, dir)
			require.NoError(t, gw.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir, FirstPlugin: input, LastPlugin: output}))
			t.Cleanup(func() { require.NoError(t, gw.Shutdown(context.Background())) })
			code, body := call(t, gw, fmt.Sprintf(`{"model":"openai/outer","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream), true)
			require.Equal(t, http.StatusOK, code, body)
			require.Contains(t, body, "original answer", "the outer response must survive judge evaluation")
			require.EqualValues(t, 1, judgeCalls.Load(), "judge replies must not be guarded again")
		})
	}
}
