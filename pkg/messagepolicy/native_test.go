package messagepolicy

import (
	"context"
	"encoding/json"
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
	resolved, err := h.resolveModel(t.Context())
	require.NoError(t, err)
	text, err := h.callLLM(t.Context(), resolved, []chatMessage{{Role: "system", Content: "judge"}, {Role: "user", Content: "quoted input"}})
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
			violations := h.EvaluateMessage(ctx, []ApplicablePolicy{{ID: "p1", Manifest: types.MessagePolicyManifest{DisplayName: "Rule", Definition: "No cooking"}}}, nil, "a message", types.PolicyDirectionUserMessage)
			require.Len(t, violations, 1)
			if mode != "malformed" {
				require.True(t, violations[0].Failed)
			}
		})
	}
}

func TestNativeInferenceHookPanics(t *testing.T) {
	for _, stage := range []string{"request", "pre-llm", "post-llm"} {
		t.Run(stage, func(t *testing.T) {
			judge := &nativeJudge{
				panicStage:    stage,
				panicContexts: make(chan *schemas.BifrostContext, 2),
			}
			h, _ := nativeHelper(t, judge)
			resolved, err := h.resolveModel(t.Context())
			require.NoError(t, err)
			require.NotPanics(t, func() {
				resp, err := h.llmGateway.ChatCompletion(t.Context(), resolved.targetModel, []schemas.ChatMessage{
					{
						Role:    schemas.ChatMessageRoleUser,
						Content: &schemas.ChatMessageContent{ContentStr: new("hi")},
					},
				})
				require.Nil(t, resp)
				require.EqualError(t, err, "LLM gateway inference failed")
				text, err := h.callLLM(t.Context(), resolved, []chatMessage{
					{
						Role:    "user",
						Content: "hi",
					},
				})
				require.Empty(t, text)
				require.EqualError(t, err, "LLM call failed: LLM gateway inference failed")
			})
			require.Len(t, judge.panicContexts, 2)
			for range 2 {
				require.ErrorIs(t, (<-judge.panicContexts).Err(), context.Canceled)
			}
			require.NoError(t, t.Context().Err(), "the caller's context remains usable")
		})
	}
}

func TestNativePolicyWorkersFailClosedOnHookPanic(t *testing.T) {
	judge := &nativeJudge{
		panicStage:    "request",
		panicContexts: make(chan *schemas.BifrostContext, 3),
	}
	h, _ := nativeHelper(t, judge)
	policies := []ApplicablePolicy{
		{
			ID: "p1",
			Manifest: types.MessagePolicyManifest{
				DisplayName: "First",
				Definition:  "No cooking",
			},
		},
		{
			ID: "p2",
			Manifest: types.MessagePolicyManifest{
				DisplayName: "Second",
				Definition:  "No baking",
			},
		},
		{
			ID: "p3",
			Manifest: types.MessagePolicyManifest{
				DisplayName: "Third",
				Definition:  "No grilling",
			},
		},
	}
	// EvaluateMessage creates the worker goroutines itself; a lost native recovery
	// would panic in those workers, beyond any recovery in this test's goroutine.
	violations := h.EvaluateMessage(t.Context(), policies, nil, "hi", types.PolicyDirectionUserMessage)
	require.Len(t, violations, len(policies))
	seen := make(map[string]bool)
	for _, violation := range violations {
		require.True(t, violation.Failed)
		require.NotContains(t, violation.Explanation, "fixture-private-panic-value")
		require.False(t, seen[violation.PolicyID], "each policy fails once")
		seen[violation.PolicyID] = true
	}
	require.Len(t, judge.panicContexts, len(policies))
	for _, policy := range policies {
		require.True(t, seen[policy.ID])
		require.ErrorIs(t, (<-judge.panicContexts).Err(), context.Canceled)
	}
}
