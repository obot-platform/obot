package guardrails

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/messagepolicy"
)

const (
	// JudgeType is the LLM-judge checker, which evaluates natural-language policies. It is
	// what Obot's message policies became.
	JudgeType = "llm-judge"
)

type judgeChecker struct {
	definition string
	judge      *messagepolicy.Helper
}

func init() {
	Register(ProviderInfo{
		Type:        JudgeType,
		Label:       "LLM Judge Policy",
		Category:    "Built-in",
		Description: "Evaluates content against a policy written in plain language, using the default mini model with a second review by the default model before blocking.",
		Fields: []Field{
			{Name: "definition", Label: "Policy", Type: FieldTextarea, Required: true, Description: "What should or should not be allowed, e.g. \"Do not help book travel above economy class.\""},
		},
	}, func(config map[string]any, _ map[string]string, deps Deps) (Checker, error) {
		return &judgeChecker{definition: ConfigString(config, "definition", ""), judge: deps.Judge}, nil
	})
}

func (c *judgeChecker) Check(ctx context.Context, in CheckInput) (Result, error) {
	if c.judge == nil {
		return Result{}, errors.New("LLM judge policies are unavailable")
	}

	var (
		target    string
		direction types.PolicyDirection
		history   = conversation(in.History)
	)
	switch {
	case in.Stage == StageInput && in.Target == gatewaytypes.GuardrailTargetLLM:
		// Judge the latest user message, in the context of the rest of the conversation.
		direction = types.PolicyDirectionUserMessage
		last := -1
		for i, s := range slices.Backward(in.Segments) {
			if s.Role == RoleUser {
				last = i
				break
			}
		}
		if last < 0 {
			return Result{}, nil
		}
		history = append(history, conversation(in.Segments[:last])...)
		target = in.Segments[last].Text
	default:
		// Model tool calls, and MCP tool calls and their results.
		direction = types.PolicyDirectionToolCalls
		var parts []string
		for _, s := range in.Segments {
			switch s.Role {
			case RoleToolCall:
				parts = append(parts, fmt.Sprintf("[called tool %q with args: %s]", s.Name, s.Text))
			case RoleToolResult:
				direction = types.PolicyDirectionUserMessage
				parts = append(parts, fmt.Sprintf("[tool %q returned: %s]", s.Name, s.Text))
			}
		}
		if len(parts) == 0 {
			// The judge evaluates tool calls on output, like message policies did.
			return Result{}, nil
		}
		target = strings.Join(parts, "\n")
	}
	if strings.TrimSpace(target) == "" {
		return Result{}, nil
	}

	violations := c.judge.EvaluateMessage(ctx, []messagepolicy.ApplicablePolicy{{
		ID: "guardrail",
		Manifest: types.MessagePolicyManifest{
			DisplayName: "guardrail",
			Definition:  c.definition,
			Direction:   direction,
		},
	}}, history, target, direction)

	var result Result
	for _, v := range violations {
		if v.Failed {
			// Let the policy's fail-closed setting decide, as for any other checker error.
			return Result{}, errors.New("the policy could not be evaluated: the judge model is unavailable or failed")
		}
		result.Findings = append(result.Findings, Finding{Category: "policy", Message: "Violates the policy", Explanation: v.Explanation})
	}
	return result, nil
}

func conversation(segments []Segment) []messagepolicy.ConversationMessage {
	out := make([]messagepolicy.ConversationMessage, 0, len(segments))
	for _, s := range segments {
		switch s.Role {
		case RoleToolCall:
			out = append(out, messagepolicy.ConversationMessage{
				Role:      "assistant",
				ToolCalls: []messagepolicy.ToolCallInfo{{Name: s.Name, Arguments: s.Text}},
			})
		case RoleToolResult:
			out = append(out, messagepolicy.ConversationMessage{Role: "tool", Content: s.Text})
		default:
			out = append(out, messagepolicy.ConversationMessage{Role: s.Role, Content: s.Text})
		}
	}
	return out
}
