package guardrails

import (
	"encoding/json"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
)

// TestStreamRewriteToolArgumentsOnce checks that a held Responses stream delivers a tool call's
// arguments exactly once. The output item that starts the call carries empty arguments, and
// must keep them: the Anthropic Messages converter streams a starting item's arguments before
// its deltas, so filling them in would send the arguments twice.
func TestStreamRewriteToolArgumentsOnce(t *testing.T) {
	itemID, args := "fc_1", `{"city":"Paris"}`
	functionCall := schemas.ResponsesMessageTypeFunctionCall
	item := func(arguments string) *schemas.ResponsesMessage {
		return &schemas.ResponsesMessage{
			ID:   new(itemID),
			Type: &functionCall,
			ResponsesToolMessage: &schemas.ResponsesToolMessage{
				Name:      new("get_weather"),
				Arguments: new(arguments),
			},
		}
	}
	events := []*schemas.BifrostResponsesStreamResponse{
		{Type: schemas.ResponsesStreamResponseTypeOutputItemAdded, ItemID: new(itemID), Item: item("")},
		{Type: schemas.ResponsesStreamResponseTypeFunctionCallArgumentsDelta, ItemID: new(itemID), Delta: new(`{"city":`)},
		{Type: schemas.ResponsesStreamResponseTypeFunctionCallArgumentsDelta, ItemID: new(itemID), Delta: new(`"Paris"}`)},
		{Type: schemas.ResponsesStreamResponseTypeFunctionCallArgumentsDone, ItemID: new(itemID), Arguments: new(args)},
		{Type: schemas.ResponsesStreamResponseTypeOutputItemDone, ItemID: new(itemID), Item: item(args)},
	}

	st := newStreamState()
	for i, ev := range events {
		resp := &schemas.BifrostResponse{ResponsesStreamResponse: ev}
		st.observe(resp)
		events[i] = st.blank(resp).ResponsesStreamResponse
	}
	final := st.finalText(Decision{})
	emitted := map[string]bool{}
	var out []*schemas.BifrostResponsesStreamResponse
	for _, ev := range events {
		out = append(out, st.rewriteResponsesEvent(ev, final, emitted))
	}

	if got := *out[0].Item.Arguments; got != "" {
		t.Errorf("output_item.added arguments = %q, want empty", got)
	}
	var deltas string
	for _, ev := range out[1:3] {
		deltas += *ev.Delta
	}
	if deltas != args {
		t.Errorf("deltas = %q, want %q", deltas, args)
	}
	if got := *out[3].Arguments; got != args {
		t.Errorf("function_call_arguments.done arguments = %q, want %q", got, args)
	}
	if got := *out[4].Item.Arguments; got != args {
		t.Errorf("output_item.done arguments = %q, want %q", got, args)
	}
}

func TestHeldResponsesStreamCompletionText(t *testing.T) {
	for _, redacted := range []bool{false, true} {
		t.Run(map[bool]string{false: "allowed", true: "redacted"}[redacted], func(t *testing.T) {
			// Replay receives blanked chunks, while the final response is still original.
			const fixture = `[
				{"type":"response.output_item.added","item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":""}]}},
				{"type":"response.content_part.added","item_id":"msg_1","part":{"type":"output_text","text":""}},
				{"type":"response.output_text.delta","item_id":"msg_1","delta":"Hello, "},
				{"type":"response.output_text.delta","item_id":"msg_1","delta":"世界!"},
				{"type":"response.output_text.done","item_id":"msg_1","text":"Hello, 世界!"},
				{"type":"response.content_part.done","item_id":"msg_1","part":{"type":"output_text","text":"Hello, 世界!"}},
				{"type":"response.output_item.done","item":{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello, 世界!"}]}},
				{"type":"response.completed","response":{"output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello, 世界!"}]}]}}
			]`
			var events []*schemas.BifrostResponsesStreamResponse
			require.NoError(t, json.Unmarshal([]byte(fixture), &events))
			st := newStreamState()
			for i, ev := range events {
				resp := &schemas.BifrostResponse{ResponsesStreamResponse: ev}
				st.observe(resp)
				if i < len(events)-1 {
					events[i] = st.blank(resp).ResponsesStreamResponse
				}
			}
			want := "Hello, 世界!"
			decision := Decision{}
			if redacted {
				want = "[REDACTED]"
				decision = Decision{Redacted: true, Segments: []Segment{{Role: RoleAssistant, Text: want}}}
			}
			final := st.finalText(decision)
			emitted := map[string]bool{}
			for i, ev := range events {
				events[i] = st.rewriteResponsesEvent(ev, final, emitted)
			}
			require.Equal(t, "", *events[0].Item.Content.ContentBlocks[0].Text)
			require.Equal(t, "", *events[1].Part.Text)
			require.Equal(t, want, *events[2].Delta+*events[3].Delta)
			require.Equal(t, want, *events[4].Text, "output_text.done")
			require.Equal(t, want, *events[5].Part.Text, "content_part.done")
			require.Equal(t, want, *events[6].Item.Content.ContentBlocks[0].Text, "output_item.done")
			require.Equal(t, want, *events[7].Response.Output[0].Content.ContentBlocks[0].Text, "response.completed")
		})
	}
}
