package mcptester

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
)

type normalizationFixture struct {
	Name     string                       `json:"name"`
	Dialect  llmtypes.Dialect             `json:"dialect"`
	Tools    []types.MCPTesterTool        `json:"tools"`
	Stream   string                       `json:"stream"`
	Expected []types.MCPTesterStreamEvent `json:"expected"`
}

func TestTerminalEventContractFixture(t *testing.T) {
	file, err := os.Open("testdata/terminal_events.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close fixture: %v", err)
		}
	})

	var events []types.MCPTesterStreamEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event types.MCPTesterStreamEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("unmarshal terminal event %d: %v", len(events)+1, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	if len(events) != 6 {
		t.Fatalf("terminal event count = %d, want 6", len(events))
	}
	if len(events[1].Calls) != 2 || events[1].Calls[0].ID != "call-1" || events[1].Calls[1].ID != "call-2" {
		t.Fatalf("multiple tool-call event = %#v, want two ordered calls", events[1])
	}
	if events[2].Error == nil || events[2].Error.Code != types.MCPTesterErrorProvider {
		t.Fatalf("provider event = %#v", events[2])
	}
	if events[3].Error == nil || events[3].Error.Code != types.MCPTesterErrorUnsupportedResponse {
		t.Fatalf("malformed event = %#v", events[3])
	}
	if events[4].Error == nil || events[4].Error.Code != types.MCPTesterErrorCancelled {
		t.Fatalf("cancellation event = %#v", events[4])
	}
	if events[5].Error == nil || events[5].Error.Code != types.MCPTesterErrorPolicyDenied {
		t.Fatalf("policy-denial event = %#v", events[5])
	}
}

func TestNormalizeStreamFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/normalization.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []normalizationFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			var events []types.MCPTesterStreamEvent
			err := NormalizeStream(t.Context(), fixture.Dialect, fixture.Tools, strings.NewReader(fixture.Stream), func(event types.MCPTesterStreamEvent) error {
				events = append(events, event)
				return nil
			})
			if err != nil {
				t.Fatalf("NormalizeStream() error = %v", err)
			}

			got, err := json.Marshal(events)
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(fixture.Expected)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("events = %s, want %s", got, want)
			}
		})
	}
}

func TestNormalizeStreamMalformedEvent(t *testing.T) {
	var events []types.MCPTesterStreamEvent
	err := NormalizeStream(t.Context(), llmtypes.DialectOpenAIResponses, nil, strings.NewReader("data: {not-json}\n\n"), func(event types.MCPTesterStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("NormalizeStream() error = %v", err)
	}
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != types.MCPTesterErrorUnsupportedResponse {
		t.Fatalf("events = %#v, want unsupported response error", events)
	}
}

func TestNormalizeStreamCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var events []types.MCPTesterStreamEvent
	err := NormalizeStream(ctx, llmtypes.DialectAnthropicMessages, nil, strings.NewReader(""), func(event types.MCPTesterStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("NormalizeStream() error = %v", err)
	}
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != types.MCPTesterErrorCancelled {
		t.Fatalf("events = %#v, want cancellation error", events)
	}
}

func TestNormalizeStreamRejectsCallsOutsideTheRequestSnapshot(t *testing.T) {
	for _, test := range []struct {
		name   string
		tools  []types.MCPTesterTool
		stream []string
		want   string
	}{
		{
			name:  "tool not advertised",
			tools: []types.MCPTesterTool{{Name: "lookup"}},
			stream: []string{
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"delete","arguments":"{}"}}`,
				`data: {"type":"response.completed"}`,
			},
			want: "not provided in this request",
		},
		{
			name:  "no tools advertised",
			tools: nil,
			stream: []string{
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"lookup","arguments":"{}"}}`,
				`data: {"type":"response.completed"}`,
			},
			want: "not provided in this request",
		},
		{
			name:  "duplicate call ID",
			tools: []types.MCPTesterTool{{Name: "lookup"}},
			stream: []string{
				`data: {"type":"response.output_item.done","output_index":0,"item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"lookup","arguments":"{}"}}`,
				`data: {"type":"response.output_item.done","output_index":1,"item":{"id":"item-2","call_id":"call-1","type":"function_call","name":"lookup","arguments":"{}"}}`,
				`data: {"type":"response.completed"}`,
			},
			want: "duplicate tool call ID",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []types.MCPTesterStreamEvent
			err := NormalizeStream(t.Context(), llmtypes.DialectOpenAIResponses, test.tools, strings.NewReader(strings.Join(test.stream, "\n\n")), func(event types.MCPTesterStreamEvent) error {
				events = append(events, event)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range events {
				if event.Type == types.MCPTesterStreamEventToolCalls {
					t.Fatalf("events = %#v, want no tool calls surfaced", events)
				}
			}
			last := events[len(events)-1]
			if last.Error == nil || last.Error.Code != types.MCPTesterErrorUnsupportedResponse {
				t.Fatalf("last event = %#v, want unsupported response error", last)
			}
			if !strings.Contains(last.Error.Message, test.want) {
				t.Fatalf("error message = %q, want it to contain %q", last.Error.Message, test.want)
			}
		})
	}
}

func TestNormalizeStreamSurfacesRefusalsAsText(t *testing.T) {
	for _, test := range []struct {
		name    string
		dialect llmtypes.Dialect
		stream  []string
	}{
		{
			name:    "responses refusal delta",
			dialect: llmtypes.DialectOpenAIResponses,
			stream: []string{
				`data: {"type":"response.refusal.delta","delta":"I cannot help with that."}`,
				`data: {"type":"response.completed"}`,
			},
		},
		{
			name:    "chat completions refusal delta",
			dialect: llmtypes.DialectOpenAIChatCompletions,
			stream: []string{
				`data: {"choices":[{"delta":{"refusal":"I cannot help with that."}}]}`,
				`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var events []types.MCPTesterStreamEvent
			err := NormalizeStream(t.Context(), test.dialect, nil, strings.NewReader(strings.Join(test.stream, "\n\n")), func(event types.MCPTesterStreamEvent) error {
				events = append(events, event)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			var text string
			for _, event := range events {
				if event.Type == types.MCPTesterStreamEventTextDelta {
					text += event.Delta
				}
			}
			if text != "I cannot help with that." {
				t.Fatalf("text = %q, want the refusal surfaced as assistant text", text)
			}
		})
	}
}

func TestNormalizeStreamPolicyDenialSuppressesToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"item-1","call_id":"call-1","type":"function_call","name":"delete","arguments":"{}"}}`,
		`data: {"obot_tool_call_policy_violation":"blocked by policy"}`,
		`data: {"type":"response.completed"}`,
	}, "\n\n")

	var events []types.MCPTesterStreamEvent
	err := NormalizeStream(t.Context(), llmtypes.DialectOpenAIResponses, []types.MCPTesterTool{{Name: "delete"}}, strings.NewReader(stream), func(event types.MCPTesterStreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Error == nil || events[0].Error.Code != types.MCPTesterErrorPolicyDenied {
		t.Fatalf("events = %#v, want one policy denial", events)
	}
}
