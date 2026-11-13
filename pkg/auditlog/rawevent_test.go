package auditlog

import (
	"encoding/json"
	"testing"

	api "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	requestMarker          = `{"obot_sentry_omitted":"details.request.body"}`
	responseMarker         = `{"obot_sentry_omitted":"details.response.body"}`
	responseJSONStringMark = `{"obot_sentry_omitted":"details.response.body","encoding":"json_string"}`
)

func TestRestoreRawEvent(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		request  string
		response string
		want     string
	}{
		// Hook payloads from Obot Sentry's own fixtures, one per parser shape it marks.
		{
			name:     "claude code",
			raw:      `{"session_id":"session-1","hook_event_name":"PostToolUse","tool_name":"mcp__everything__echo","tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `,"tool_use_id":"toolu_1","mcp_server":{"name":"everything","source":"dynamic"}}`,
			request:  `{"message":"ping"}`,
			response: `[{"type":"text","text":"Echo: ping"}]`,
			want:     `{"session_id":"session-1","hook_event_name":"PostToolUse","tool_name":"mcp__everything__echo","tool_input":{"message":"ping"},"tool_response":[{"type":"text","text":"Echo: ping"}],"tool_use_id":"toolu_1","mcp_server":{"name":"everything","source":"dynamic"}}`,
		},
		{
			name:     "claude code failure, which has no output to omit",
			raw:      `{"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":` + requestMarker + `,"error":"Exit code 1"}`,
			request:  `{"command":"cat /nonexistent/file"}`,
			response: `null`,
			want:     `{"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"cat /nonexistent/file"},"error":"Exit code 1"}`,
		},
		{
			name:     "codex",
			raw:      `{"session_id":"session-1","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `,"tool_use_id":"exec-1"}`,
			request:  `{"command":"echo hi"}`,
			response: `"hi\n"`,
			want:     `{"session_id":"session-1","hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_response":"hi\n","tool_use_id":"exec-1"}`,
		},
		{
			name:     "cursor, whose output is a JSON string",
			raw:      `{"conversation_id":"conversation-1","tool_name":"Shell","tool_input":` + requestMarker + `,"tool_output":` + responseJSONStringMark + `,"duration":554.376,"hook_event_name":"postToolUse"}`,
			request:  `{"command":"echo hi","cwd":"","timeout":30000}`,
			response: `{"output":"hi\n","exitCode":0}`,
			want:     `{"conversation_id":"conversation-1","tool_name":"Shell","tool_input":{"command":"echo hi","cwd":"","timeout":30000},"tool_output":"{\"output\":\"hi\\n\",\"exitCode\":0}","duration":554.376,"hook_event_name":"postToolUse"}`,
		},
		{
			name:     "vs code and copilot cli",
			raw:      `{"hook_event_name":"PostToolUse","session_id":"session-vs","tool_use_id":"tool-vs","tool_name":"Bash","tool_input":` + requestMarker + `,"tool_result":` + responseMarker + `}`,
			request:  `{"command":"echo hi"}`,
			response: `{"result_type":"success","text_result_for_llm":"hi"}`,
			want:     `{"hook_event_name":"PostToolUse","session_id":"session-vs","tool_use_id":"tool-vs","tool_name":"Bash","tool_input":{"command":"echo hi"},"tool_result":{"result_type":"success","text_result_for_llm":"hi"}}`,
		},
		{
			name:     "kiro",
			raw:      `{"session_id":"kiro-session","hook_event_name":"PostToolUse","cwd":"/work/repo","tool_name":"execute_bash","tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `}`,
			request:  `{"command":"go test ./..."}`,
			response: `{"exitCode":0,"stdout":"ok"}`,
			want:     `{"session_id":"kiro-session","hook_event_name":"PostToolUse","cwd":"/work/repo","tool_name":"execute_bash","tool_input":{"command":"go test ./..."},"tool_response":{"exitCode":0,"stdout":"ok"}}`,
		},
		// Behaviour around the markers.
		{
			name:     "cursor integers beyond float64 precision keep their digits",
			raw:      `{"tool_output":` + responseJSONStringMark + `}`,
			response: `{"id":9007199254740993,"ratio":0.1000000000000000055}`,
			want:     `{"tool_output":"{\"id\":9007199254740993,\"ratio\":0.1000000000000000055}"}`,
		},
		{
			name:     "a cursor escape after an escaped backslash stays literal text",
			raw:      `{"tool_output":` + responseJSONStringMark + `}`,
			response: `{"path":"C:\\u0026","both":"\\\u0026"}`,
			want:     `{"tool_output":"{\"path\":\"C:\\\\u0026\",\"both\":\"\\\\&\"}"}`,
		},
		{
			name:     "a stored body that was reformatted is compacted into the cursor string",
			raw:      `{"tool_output":` + responseJSONStringMark + `}`,
			response: `{"output": "hi", "exitCode": 0}`,
			want:     `{"tool_output":"{\"output\":\"hi\",\"exitCode\":0}"}`,
		},
		{
			name:    "whitespace and every other byte of the raw event are kept",
			raw:     "{ \"a\" : 1e3 ,\n  \"tool_input\" :  { \"obot_sentry_omitted\" : \"details.request.body\" } , \"b\":\"\\u00e9\" }",
			request: `{"command":"x"}`,
			want:    "{ \"a\" : 1e3 ,\n  \"tool_input\" :  {\"command\":\"x\"} , \"b\":\"\\u00e9\" }",
		},
		{
			name:     "null and empty bodies are restored",
			raw:      `{"tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `}`,
			request:  `null`,
			response: `{}`,
			want:     `{"tool_input":null,"tool_response":{}}`,
		},
		{
			name:    "a marker whose body was not recorded or was blanked stays",
			raw:     `{"tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `}`,
			request: `{"command":"x"}`,
			want:    `{"tool_input":{"command":"x"},"tool_response":` + responseMarker + `}`,
		},
		{
			name:    "a marker whose body is not valid JSON stays",
			raw:     `{"tool_input":` + requestMarker + `}`,
			request: `{not json`,
			want:    `{"tool_input":` + requestMarker + `}`,
		},
		{
			name:     "objects that are not exactly a marker are left alone",
			raw:      `{"a":{"obot_sentry_omitted":"details.request.body","other":true},"b":{"obot_sentry_omitted":"details.response.body","encoding":"base64"},"c":{"obot_sentry_omitted":"details.somewhere.else"},"d":[` + requestMarker + `],"e":{"nested":` + requestMarker + `}}`,
			request:  `"req"`,
			response: `"resp"`,
			want:     `{"a":{"obot_sentry_omitted":"details.request.body","other":true},"b":{"obot_sentry_omitted":"details.response.body","encoding":"base64"},"c":{"obot_sentry_omitted":"details.somewhere.else"},"d":[` + requestMarker + `],"e":{"nested":` + requestMarker + `}}`,
		},
		{
			name:    "a raw event that is not an object is unchanged",
			raw:     `[` + requestMarker + `]`,
			request: `"req"`,
			want:    `[` + requestMarker + `]`,
		},
		{
			name:     "a raw event with no marker is unchanged",
			raw:      `{"tool_input":{"command":"x"},"tool_response":"y"}`,
			request:  `{"command":"x"}`,
			response: `"y"`,
			want:     `{"tool_input":{"command":"x"},"tool_response":"y"}`,
		},
		{
			name: "an empty raw event stays empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := restoreRawEvent(rawOrNil(tt.raw), rawOrNil(tt.request), rawOrNil(tt.response))
			if string(got) != tt.want {
				t.Fatalf("restoreRawEvent() =\n%s\nwant\n%s", got, tt.want)
			}
			if len(got) > 0 && !json.Valid(got) {
				t.Fatalf("restoreRawEvent() returned invalid JSON: %s", got)
			}
		})
	}
}

func TestPresentLocalAgentRestoresRawEvent(t *testing.T) {
	log := gatewaytypes.MCPAuditLog{
		SourceType: api.AuditLogSourceTypeLocalAgentToolCall,
		LocalAgentToolCallFields: &gatewaytypes.LocalAgentToolCallAuditLogFields{
			RequestBody:  json.RawMessage(`{"command":"uname -a"}`),
			ResponseBody: json.RawMessage(`"Linux\n"`),
			RawEvent:     json.RawMessage(`{"tool_input":` + requestMarker + `,"tool_response":` + responseMarker + `}`),
		},
	}

	event := Present(log, PresentOptions{IncludeDetails: true})

	want := `{"tool_input":{"command":"uname -a"},"tool_response":"Linux\n"}`
	if string(event.Details.RawEvent) != want {
		t.Fatalf("raw event = %s, want %s", event.Details.RawEvent, want)
	}
	if string(event.Details.Request.Body) != `{"command":"uname -a"}` || string(event.Details.Response.Body) != `"Linux\n"` {
		t.Fatalf("request and response bodies changed: %s, %s", event.Details.Request.Body, event.Details.Response.Body)
	}
	if string(log.LocalAgentToolCallFields.RawEvent) != `{"tool_input":`+requestMarker+`,"tool_response":`+responseMarker+`}` {
		t.Fatalf("Present mutated the stored raw event: %s", log.LocalAgentToolCallFields.RawEvent)
	}
	if _, err := json.Marshal(event); err != nil {
		t.Fatalf("presented event does not marshal: %v", err)
	}
}

// TestRestoreRawEventCursorSubmissionRoundTrip sends a Cursor hook payload the way Obot Sentry
// does: it marks the output and submits the event with encoding/json, which escapes <, >, &,
// U+2028 and U+2029 inside the body. Restoring the stored event must give back Cursor's string.
func TestRestoreRawEventCursorSubmissionRoundTrip(t *testing.T) {
	outputs := []string{
		`{"output":"a && b <x> -> c\n","exitCode":0}`,
		`{"html":"<a href=\"x\">&amp;</a>"}`,
		"{\"text\":\"line\u2028para\u2029end\"}",
		`{"path":"C:\\u0026 and \\&"}`,
		`{"id":9007199254740993,"empty":"","nested":{"list":[1,"<",{"k":">"}]}}`,
	}

	for _, output := range outputs {
		t.Run(output, func(t *testing.T) {
			original := map[string]string{"tool_name": "Shell", "tool_output": output}

			// What Obot Sentry submits, through encoding/json.
			submitted, err := json.Marshal(struct {
				RawEvent json.RawMessage `json:"rawEvent"`
				Body     json.RawMessage `json:"body"`
			}{
				RawEvent: json.RawMessage(`{"tool_name":"Shell","tool_output":` + responseJSONStringMark + `}`),
				Body:     json.RawMessage(output),
			})
			if err != nil {
				t.Fatal(err)
			}
			// What Obot stores: the submitted bytes of each field.
			var stored struct {
				RawEvent json.RawMessage `json:"rawEvent"`
				Body     json.RawMessage `json:"body"`
			}
			if err := json.Unmarshal(submitted, &stored); err != nil {
				t.Fatal(err)
			}

			restored := restoreRawEvent(stored.RawEvent, nil, stored.Body)

			var got map[string]string
			if err := json.Unmarshal(restored, &got); err != nil {
				t.Fatalf("restored raw event %s: %v", restored, err)
			}
			if got["tool_output"] != original["tool_output"] || got["tool_name"] != original["tool_name"] {
				t.Fatalf("tool_output = %q, want %q", got["tool_output"], original["tool_output"])
			}
		})
	}
}

func rawOrNil(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}
