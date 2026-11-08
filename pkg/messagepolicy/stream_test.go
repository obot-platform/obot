package messagepolicy

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestStreamAccumulatorAssemblesTextAndToolCalls(t *testing.T) {
	s := func(v string) *string { return &v }
	var acc streamAccumulator
	deltas := []*schemas.ChatStreamResponseChoiceDelta{
		{Content: s("Let me ")},
		{Content: s("check.")},
		{ToolCalls: []schemas.ChatAssistantMessageToolCall{{Index: 0, ID: s("call_1"), Function: schemas.ChatAssistantMessageToolCallFunction{Name: s("llm_usage"), Arguments: `{"da`}}}},
		{ToolCalls: []schemas.ChatAssistantMessageToolCall{{Index: 0, Function: schemas.ChatAssistantMessageToolCallFunction{Arguments: `ys":7}`}}}},
		{ToolCalls: []schemas.ChatAssistantMessageToolCall{{Index: 1, ID: s("call_2"), Function: schemas.ChatAssistantMessageToolCallFunction{Name: s("adoption_summary"), Arguments: `{}`}}}},
	}
	var streamed string
	for _, d := range deltas {
		streamed += acc.add(d)
	}
	if streamed != "Let me check." {
		t.Fatalf("streamed %q", streamed)
	}
	m := acc.message()
	if *m.Content.ContentStr != "Let me check." {
		t.Fatalf("text %q", *m.Content.ContentStr)
	}
	if len(m.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %+v", m.ToolCalls)
	}
	if *m.ToolCalls[0].ID != "call_1" || *m.ToolCalls[0].Function.Name != "llm_usage" || m.ToolCalls[0].Function.Arguments != `{"days":7}` {
		t.Fatalf("first call %+v", m.ToolCalls[0])
	}
	if *m.ToolCalls[1].Function.Name != "adoption_summary" {
		t.Fatalf("second call %+v", m.ToolCalls[1])
	}
}
