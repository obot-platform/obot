package messagepolicy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type nativeCodec struct{}

type nativeJudge struct {
	panicStage    string
	panicContexts chan *schemas.BifrostContext
	text          string
	fail          bool
	cancel        bool
	model         string
	messages      []schemas.ChatMessage
}

func (nativeCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (nativeCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func (*nativeJudge) GetName() string { return "native-judge-fixture" }
func (*nativeJudge) Cleanup() error  { return nil }
func (p *nativeJudge) panicAt(ctx *schemas.BifrostContext, stage string) {
	if p.panicStage == stage {
		p.panicContexts <- ctx
		panic("fixture-private-panic-value")
	}
}

func (p *nativeJudge) PreRequestHook(ctx *schemas.BifrostContext, _ *schemas.BifrostRequest) error {
	p.panicAt(ctx, "request")
	return nil
}
func (p *nativeJudge) PostLLMHook(ctx *schemas.BifrostContext, r *schemas.BifrostResponse, e *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	p.panicAt(ctx, "post-llm")
	return r, e, nil
}
func (p *nativeJudge) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	p.panicAt(ctx, "pre-llm")
	p.model = req.ChatRequest.Model
	p.messages = req.ChatRequest.Input
	if p.cancel {
		<-ctx.Done()
	}
	if p.fail || p.cancel {
		return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{Error: &schemas.ErrorField{Message: "judge unavailable"}}}, nil
	}
	var resp schemas.BifrostChatResponse
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": p.text}}}})
	_ = json.Unmarshal(body, &resp)
	return req, &schemas.LLMPluginShortCircuit{Response: &schemas.BifrostResponse{ChatResponse: &resp}}, nil
}

func nativeHelper(t *testing.T, judge *nativeJudge) (*Helper, kclient.Client) {
	t.Helper()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, nativeCodec{})
	require.NoError(t, err)
	// A configured custom provider with a namespaced model must survive native parsing.
	require.NoError(t, store.CreateProvider(t.Context(), "judge-custom", llmcoreclient.Provider{Config: schemas.ProviderConfig{CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}}}))
	engine := llmengine.New()
	require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: t.TempDir(), LastPlugin: judge}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	model := &v1.Model{Name: "m1-judge", Namespace: system.DefaultNamespace, Spec: v1.ModelSpec{Manifest: types.ModelManifest{Active: true, ModelProvider: system.LLMGatewayModelProvider, TargetModel: "judge-custom/team/model"}}}
	require.NoError(t, storage.Create(t.Context(), model))
	for _, name := range []types.DefaultModelAliasType{types.DefaultModelAliasTypeLLMMini, types.DefaultModelAliasTypeLLM} {
		require.NoError(t, storage.Create(t.Context(), &v1.Alias{Name: alias.KeyFromScopeID("Model", string(name)), Spec: v1.AliasSpec{TargetKind: "Model", TargetNamespace: system.DefaultNamespace, TargetName: model.Name}}))
	}
	return NewHelper(storage, engine), storage
}

func TestNativePolicyInference(t *testing.T) {
	judge := &nativeJudge{text: "yes"}
	h, _ := nativeHelper(t, judge)
	// Retirement in another replica must not affect this replica's model parsing.
	schemas.UnregisterKnownProvider("judge-custom")
	resolved, err := h.resolveModelByAlias(t.Context(), types.DefaultModelAliasTypeLLMMini)
	require.NoError(t, err)
	text, err := h.callLLM(t.Context(), resolved, []schemas.ChatMessage{
		{Role: schemas.ChatMessageRoleSystem, Content: &schemas.ChatMessageContent{ContentStr: new("judge")}},
		{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("quoted input")}},
	})
	require.NoError(t, err)
	require.Equal(t, "yes", text)
	require.Equal(t, "team/model", judge.model)
	require.Equal(t, "judge", *judge.messages[0].Content.ContentStr)
	require.Equal(t, "quoted input", *judge.messages[1].Content.ContentStr)
}

func TestNativePolicyFailuresStayClosed(t *testing.T) {
	for _, mode := range []string{"error", "empty", "malformed", "canceled", "unavailable", "missing", "inactive", "wrong provider", "empty alias"} {
		t.Run(mode, func(t *testing.T) {
			judge := &nativeJudge{text: "yes", fail: mode == "error", cancel: mode == "canceled"}
			h, storage := nativeHelper(t, judge)
			switch mode {
			case "empty":
				judge.text = ""
			case "malformed":
				judge.text = "maybe"
			case "unavailable":
				h.llmGateway = llmengine.New()
			case "missing", "inactive", "wrong provider":
				model := &v1.Model{}
				require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-judge"}, model))
				if mode == "missing" {
					require.NoError(t, storage.Delete(t.Context(), model))
				} else {
					if mode == "inactive" {
						model.Spec.Manifest.Active = false
					} else {
						model.Spec.Manifest.ModelProvider = "other"
					}
					require.NoError(t, storage.Update(t.Context(), model))
				}
			case "empty alias":
				obj := &v1.Alias{}
				require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Name: alias.KeyFromScopeID("Model", string(types.DefaultModelAliasTypeLLMMini))}, obj))
				obj.Spec.TargetKind = "DefaultModelAlias"
				require.NoError(t, storage.Update(t.Context(), obj))
				require.NoError(t, storage.Create(t.Context(), &v1.DefaultModelAlias{Name: "m1-judge", Namespace: system.DefaultNamespace}))
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			result, err := h.EvaluateMessage(ctx, types.MessagePolicyManifest{DisplayName: "Rule", Definition: "No cooking"}, "", "a message")
			require.False(t, result.Compliant)
			if mode == "malformed" {
				require.NoError(t, err)
				require.Equal(t, `Your message was blocked by the "Rule" message policy.`, result.Explanation)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestNativeInferenceHookPanics(t *testing.T) {
	for _, stage := range []string{"request", "pre-llm", "post-llm"} {
		t.Run(stage, func(t *testing.T) {
			judge := &nativeJudge{
				panicStage:    stage,
				panicContexts: make(chan *schemas.BifrostContext, 4),
			}
			h, _ := nativeHelper(t, judge)
			resolved, err := h.resolveModelByAlias(t.Context(), types.DefaultModelAliasTypeLLMMini)
			require.NoError(t, err)
			require.NotPanics(t, func() {
				resp, err := h.llmGateway.ChatCompletion(t.Context(), resolved, []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleUser,
						Content: &schemas.ChatMessageContent{ContentStr: new("hi")},
					},
				})
				require.Nil(t, resp)
				require.EqualError(t, err, "LLM gateway inference failed")
				text, err := h.callLLM(t.Context(), resolved, []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleUser,
						Content: &schemas.ChatMessageContent{ContentStr: new("hi")},
					},
				})
				require.Empty(t, text)
				require.EqualError(t, err, "LLM call failed: LLM gateway inference failed")
			})
			_, err = h.EvaluateMessage(t.Context(), types.MessagePolicyManifest{}, "", "hi")
			require.EqualError(t, err, "LLM call failed: LLM gateway inference failed")

			// Let screening deny without a hook call, so the panic occurs in full review.
			engine := h.llmGateway
			h.llmGateway = completionFunc(func(ctx context.Context, model string, messages []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
				if strings.HasPrefix(*messages[0].Content.ContentStr, "You are a policy compliance checker.") {
					return textResponse("no"), nil
				}
				return engine.ChatCompletion(ctx, model, messages)
			})
			_, err = h.EvaluateMessage(t.Context(), types.MessagePolicyManifest{}, "", "hi")
			require.EqualError(t, err, "LLM call failed: LLM gateway inference failed")

			require.Len(t, judge.panicContexts, 4)
			for range 4 {
				require.ErrorIs(t, (<-judge.panicContexts).Err(), context.Canceled)
			}
			require.NoError(t, t.Context().Err(), "the caller's context remains usable")
		})
	}
}

func TestNativeConcurrentPoliciesFailOnHookPanic(t *testing.T) {
	judge := &nativeJudge{
		panicStage:    "request",
		panicContexts: make(chan *schemas.BifrostContext, 3),
	}
	h, _ := nativeHelper(t, judge)
	policies := []types.MessagePolicyManifest{
		{
			DisplayName: "First",
			Definition:  "No cooking",
		},
		{
			DisplayName: "Second",
			Definition:  "No baking",
		},
		{
			DisplayName: "Third",
			Definition:  "No grilling",
		},
	}
	// Callers own concurrency; a lost native recovery would panic outside this goroutine.
	errors := make(chan error, len(policies))
	for _, policy := range policies {
		go func() {
			_, err := h.EvaluateMessage(t.Context(), policy, "", "hi")
			errors <- err
		}()
	}
	for range policies {
		err := <-errors
		require.EqualError(t, err, "LLM call failed: LLM gateway inference failed")
		require.NotContains(t, err.Error(), "fixture-private-panic-value")
	}
	require.Len(t, judge.panicContexts, len(policies))
	for range policies {
		require.ErrorIs(t, (<-judge.panicContexts).Err(), context.Canceled)
	}
	require.NoError(t, t.Context().Err(), "the caller's context remains usable")
}
