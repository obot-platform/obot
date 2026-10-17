package guardrails

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
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
	for _, ev := range events {
		st.observe(&schemas.BifrostResponse{ResponsesStreamResponse: ev})
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
