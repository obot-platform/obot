package builtinmcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

type fakeModel struct{ streamed bool }

type fakeStreamingModel struct{ fakeModel }

func TestEventStreamWritesServerSentEvents(t *testing.T) {
	rec := httptest.NewRecorder()
	events, err := newEventStream(rec)
	if err != nil {
		t.Fatal(err)
	}
	events.send(AssistantEvent{Type: "tool_start", Tool: "llm_usage", Arguments: json.RawMessage(`{"days":7}`)})
	events.send(AssistantEvent{Type: "delta", Text: "Spend is\nlow"})

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	frames := strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n")
	if len(frames) != 2 {
		t.Fatalf("expected 2 events, got %q", rec.Body.String())
	}
	var ev AssistantEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &ev); err != nil {
		t.Fatal(err)
	}
	// A newline in the text must stay inside the JSON, not break the event.
	if ev.Type != "delta" || ev.Text != "Spend is\nlow" {
		t.Fatalf("unexpected event %+v", ev)
	}
}

func TestTurnStreamsWhenTheModelCan(t *testing.T) {
	var got []string
	streaming := &fakeStreamingModel{}
	_, model, _ := (&Assistant{model: streaming}).turn(t.Context(), nil, nil, func(s string) { got = append(got, s) })
	if model != "streaming" || !streaming.streamed || len(got) != 1 {
		t.Fatalf("expected a streamed turn, got model %q, text %v", model, got)
	}
	_, model, _ = (&Assistant{model: &fakeModel{}}).turn(t.Context(), nil, nil, func(string) {})
	if model != "plain" {
		t.Fatalf("expected a plain turn, got %q", model)
	}
}

func (f *fakeModel) ChatTurn(context.Context, string, []schemas.ChatMessage, []schemas.ChatTool) (*schemas.ChatMessage, string, error) {
	return &schemas.ChatMessage{}, "plain", nil
}

func (f *fakeStreamingModel) ChatTurnStream(_ context.Context, _ string, _ []schemas.ChatMessage, _ []schemas.ChatTool, onText func(string)) (*schemas.ChatMessage, string, error) {
	f.streamed = true
	onText("hi")
	return &schemas.ChatMessage{}, "streaming", nil
}
