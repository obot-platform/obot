package guardrails

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/messagepolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type judgeCompletionFunc func(context.Context, string, []schemas.ChatMessage) (*schemas.BifrostChatResponse, error)

func TestJudgeConversation(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		want     string
	}{
		{
			name: "empty history",
		},
		{
			name:     "user message",
			segments: []Segment{{Role: RoleUser, Text: "Book me a first class flight to Paris"}},
			want:     "User: Book me a first class flight to Paris",
		},
		{
			name: "assistant text",
			segments: []Segment{
				{Role: RoleUser, Text: "Hello"},
				{Role: RoleAssistant, Text: "Hi there! How can I help?"},
			},
			want: "User: Hello\nAssistant: Hi there! How can I help?",
		},
		{
			name: "assistant tool calls",
			segments: []Segment{
				{Role: RoleUser, Text: "Book a flight"},
				{Role: RoleToolCall, Name: "book_flight", Text: `{"destination": "Paris", "class": "first"}`},
			},
			want: "User: Book a flight\nAssistant: [called tool \"book_flight\" with args: {\"destination\": \"Paris\", \"class\": \"first\"}]",
		},
		{
			name: "assistant text and tool calls",
			segments: []Segment{
				{Role: RoleAssistant, Text: "Let me book that for you."},
				{Role: RoleToolCall, Name: "book_flight", Text: `{"destination": "Paris"}`},
			},
			want: "Assistant: Let me book that for you.\nAssistant: [called tool \"book_flight\" with args: {\"destination\": \"Paris\"}]",
		},
		{
			name: "tool output redacted",
			segments: []Segment{
				{Role: RoleToolCall, Name: "search", Text: `{"query": "flights"}`},
				{Role: RoleToolResult, Text: "Here are 5 flights..."},
			},
			want: "Assistant: [called tool \"search\" with args: {\"query\": \"flights\"}]\nTool: [tool output redacted]",
		},
		{
			name: "system and unknown roles excluded",
			segments: []Segment{
				{Role: RoleSystem, Text: "You are a helpful assistant."},
				{Role: "developer", Text: "private instructions"},
				{Role: "unknown", Text: "unknown content"},
				{Role: RoleUser, Text: "Hello"},
			},
			want: "User: Hello",
		},
		{
			name: "full conversation",
			segments: []Segment{
				{Role: RoleSystem, Text: "You are a travel agent."},
				{Role: RoleUser, Text: "Find me a flight to NYC"},
				{Role: RoleAssistant, Text: "I'll search for flights."},
				{Role: RoleToolCall, Name: "search_flights", Text: `{"to": "NYC"}`},
				{Role: RoleToolResult, Text: "<big json response>"},
				{Role: RoleAssistant, Text: "I found several options."},
				{Role: RoleUser, Text: "Book the cheapest one in first class"},
			},
			want: "User: Find me a flight to NYC\n" +
				"Assistant: I'll search for flights.\n" +
				"Assistant: [called tool \"search_flights\" with args: {\"to\": \"NYC\"}]\n" +
				"Tool: [tool output redacted]\n" +
				"Assistant: I found several options.\n" +
				"User: Book the cheapest one in first class",
		},
		{
			name: "empty segment text",
			segments: []Segment{
				{Role: RoleUser},
				{Role: RoleAssistant},
				{Role: RoleToolCall},
				{Role: RoleToolResult},
			},
			want: "User: \nAssistant: [called tool \"\" with args: ]\nTool: [tool output redacted]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, conversation(tt.segments))
		})
	}
}

func (f judgeCompletionFunc) ChatCompletion(ctx context.Context, model string, messages []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
	return f(ctx, model, messages)
}

func checkerJudge(t *testing.T, complete judgeCompletionFunc) *messagepolicy.Helper {
	t.Helper()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	model := &v1.Model{Name: "judge", Namespace: system.DefaultNamespace, Spec: v1.ModelSpec{Manifest: apitypes.ModelManifest{
		Active:        true,
		ModelProvider: system.LLMGatewayModelProvider,
		TargetModel:   "judge",
	}}}
	require.NoError(t, storage.Create(t.Context(), model))
	for _, name := range []apitypes.DefaultModelAliasType{apitypes.DefaultModelAliasTypeLLMMini, apitypes.DefaultModelAliasTypeLLM} {
		require.NoError(t, storage.Create(t.Context(), &v1.Alias{Name: alias.KeyFromScopeID("Model", string(name)), Spec: v1.AliasSpec{
			TargetKind:      "Model",
			TargetNamespace: system.DefaultNamespace,
			TargetName:      model.Name,
		}}))
	}
	return messagepolicy.NewHelper(storage, complete)
}

func judgeResponse(text string) *schemas.BifrostChatResponse {
	return &schemas.BifrostChatResponse{Choices: []schemas.BifrostResponseChoice{{ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{Message: &schemas.ChatMessage{
		Role:    schemas.ChatMessageRoleAssistant,
		Content: &schemas.ChatMessageContent{ContentStr: new(text)},
	}}}}}
}

func TestJudgeCheckHistoryAndTargets(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		stage      string
		segments   []Segment
		wantTarget string
		wantPrefix string
	}{
		{
			name:   "latest user with preceding context",
			target: types.GuardrailTargetLLM,
			stage:  StageInput,
			segments: []Segment{
				{Role: RoleSystem, Text: "private system"},
				{Role: RoleUser, Text: "earlier user"},
				{Role: RoleToolCall, Name: "search", Text: `{"q":"flights"}`},
				{Role: RoleToolResult, Text: "private result"},
				{Role: RoleUser, Text: "latest user"},
				{Role: RoleAssistant, Text: "after latest user"},
			},
			wantPrefix: "\nUser: earlier user\nAssistant: [called tool \"search\" with args: {\"q\":\"flights\"}]\nTool: [tool output redacted]",
			wantTarget: "latest user",
		},
		{
			name:   "model tool calls",
			target: types.GuardrailTargetLLM,
			stage:  StageOutput,
			segments: []Segment{
				{Role: RoleAssistant, Text: "ignored output text"},
				{Role: RoleToolCall, Name: "book", Text: `{"class":"first"}`},
				{Role: RoleToolCall, Name: "pay", Text: "{}"},
			},
			wantTarget: "[called tool \"book\" with args: {\"class\":\"first\"}]\n[called tool \"pay\" with args: {}]",
		},
		{
			name:       "MCP tool result is judged without history redaction",
			target:     types.GuardrailTargetMCP,
			stage:      StageOutput,
			segments:   []Segment{{Role: RoleToolResult, Name: "search", Text: "result to evaluate"}},
			wantTarget: "[tool \"search\" returned: result to evaluate]",
		},
		{
			name:       "MCP tool call",
			target:     types.GuardrailTargetMCP,
			stage:      StageInput,
			segments:   []Segment{{Role: RoleToolCall, Name: "search", Text: "{}"}},
			wantTarget: "[called tool \"search\" with args: {}]",
		},
		{
			name:     "no user message",
			target:   types.GuardrailTargetLLM,
			stage:    StageInput,
			segments: []Segment{{Role: RoleAssistant, Text: "hello"}},
		},
		{
			name:     "empty latest user",
			target:   types.GuardrailTargetLLM,
			stage:    StageInput,
			segments: []Segment{{Role: RoleUser, Text: "earlier"}, {Role: RoleUser, Text: " \n"}},
		},
		{
			name:     "output without tools",
			target:   types.GuardrailTargetLLM,
			stage:    StageOutput,
			segments: []Segment{{Role: RoleAssistant, Text: "hello"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			c := &judgeChecker{definition: "No cooking", judge: checkerJudge(t, func(_ context.Context, _ string, messages []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
				calls++
				require.Equal(t, "Policy: No cooking\n\n---\n\nConversation context (tool outputs redacted):\n<conversation>\nUser: history"+tt.wantPrefix+"\n</conversation>\n\n---\n\nMessage to evaluate:\n<message>\n"+tt.wantTarget+"\n</message>", *messages[1].Content.ContentStr)
				return judgeResponse("yes"), nil
			})}
			// Spare capacity exposes accidental writes past History's visible length.
			backing := make([]Segment, 12)
			backing[0] = Segment{Role: RoleUser, Text: "history"}
			backing[1] = Segment{Role: RoleAssistant, Text: "sentinel"}
			before := slices.Clone(backing)
			segmentsBefore := slices.Clone(tt.segments)
			result, err := c.Check(t.Context(), CheckInput{Target: tt.target, Stage: tt.stage, History: backing[:1], Segments: tt.segments})
			require.NoError(t, err)
			require.Empty(t, result.Findings)
			if tt.wantTarget == "" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Equal(t, before, backing)
			require.Equal(t, segmentsBefore, tt.segments)
		})
	}
}

func TestJudgeCheckResultTranslation(t *testing.T) {
	for _, mode := range []string{"approved", "override", "denied", "malformed", "mini error", "review error", "missing judge"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			c := &judgeChecker{definition: "No cooking", judge: checkerJudge(t, func(context.Context, string, []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
				calls++
				if mode == "mini error" || (mode == "review error" && calls == 2) {
					return nil, errors.New("private provider failure")
				}
				if mode == "approved" {
					return judgeResponse("yes"), nil
				}
				if calls == 1 {
					return judgeResponse("no"), nil
				}
				if mode == "override" {
					return judgeResponse("COMPLIANT"), nil
				}
				if mode == "malformed" {
					return judgeResponse("maybe"), nil
				}
				return judgeResponse("VIOLATION: Your message asks for a recipe."), nil
			})}
			if mode == "missing judge" {
				c.judge = nil
			}
			result, err := c.Check(t.Context(), CheckInput{Target: types.GuardrailTargetLLM, Stage: StageInput, Segments: []Segment{{Role: RoleUser, Text: "message"}}})
			switch mode {
			case "missing judge":
				require.EqualError(t, err, "LLM judge policies are unavailable")
				require.Empty(t, result.Findings)
			case "mini error", "review error":
				require.EqualError(t, err, "the policy could not be evaluated: the judge model is unavailable or failed")
				require.Empty(t, result.Findings)
			case "approved", "override":
				require.NoError(t, err)
				require.Empty(t, result.Findings)
			default:
				require.NoError(t, err)
				explanation := "Your message asks for a recipe."
				if mode == "malformed" {
					explanation = `Your message was blocked by the "guardrail" message policy.`
				}
				require.Equal(t, []Finding{{Category: "policy", Message: "Violates the policy", Explanation: explanation}}, result.Findings)
			}
		})
	}
}

func TestJudgeFailureUsesPolicyFailClosed(t *testing.T) {
	for _, failClosed := range []bool{false, true} {
		p := policy("judge-policy", types.GuardrailActionBlock, "judge")
		p.FailClosed = failClosed
		store := &fakeStore{
			policies:   []types.GuardrailPolicy{p},
			guardrails: []types.Guardrail{guardrail("judge", JudgeType, map[string]any{"definition": "No cooking"})},
		}
		e := New(store, checkerJudge(t, func(context.Context, string, []schemas.ChatMessage) (*schemas.BifrostChatResponse, error) {
			return nil, errors.New("private provider failure")
		}))
		identity := Identity{ProfileIDs: []string{testProfile}}
		plan, err := e.Plan(t.Context(), types.GuardrailTargetLLM, StageInput, identity, nil)
		require.NoError(t, err)
		d := e.Evaluate(t.Context(), plan, Request{Target: types.GuardrailTargetLLM, Stage: StageInput, Identity: identity, Segments: []Segment{{Role: RoleUser, Text: "message"}}})
		require.Equal(t, failClosed, d.Blocked)
		require.Equal(t, []string{"error"}, outcomes(d))
		require.NotContains(t, d.Detections[0].Error, "private provider failure")
	}
}
