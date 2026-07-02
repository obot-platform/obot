package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatewayllmaudit "github.com/obot-platform/obot/pkg/gateway/llmaudit"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
)

func TestRedactedHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret"},
		"X-Api-Key":     []string{"secret"},
		"Content-Type":  []string{"application/json"},
	}

	got := redactedHeaders(headers)
	if strings.Contains(string(got), "Bearer secret") || strings.Contains(string(got), "secret") {
		t.Fatalf("expected secrets to be redacted, got %s", got)
	}
	if !strings.Contains(string(got), "application/json") {
		t.Fatalf("expected non-sensitive header to remain, got %s", got)
	}
}

func TestNewLLMAuditRecorderCapturesRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses?stream=true", nil)
	recorder := newLLMAuditRecorder(req, nil, 5<<20)

	if recorder.log.RequestPath != "/api/llm-provider/openai/v1/responses" {
		t.Fatalf("expected request path, got %q", recorder.log.RequestPath)
	}
	if recorder.log.RequestMethod != http.MethodPost {
		t.Fatalf("expected request method, got %q", recorder.log.RequestMethod)
	}
}

func TestLLMAuditRecorderStoresRedactedRequestSeparately(t *testing.T) {
	recorder := &llmAuditRecorder{}
	recorder.setRequestBody([]byte(`{"input":"original"}`))
	recorder.setRedactedRequestBody([]byte(`{"input":"redacted"}`))

	if recorder.log.RequestBody != `{"input":"original"}` {
		t.Fatalf("expected raw request body, got %q", recorder.log.RequestBody)
	}
	if recorder.log.RedactedRequestBody != `{"input":"redacted"}` {
		t.Fatalf("expected redacted request body, got %q", recorder.log.RedactedRequestBody)
	}
}

func TestLLMAuditRecorderSetOutcome(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		outcome string
		wantErr string
	}{
		{
			name:    "success even if request context is canceled after response",
			outcome: types.LLMAuditOutcomeSuccess,
		},
		{
			name:    "actual context cancellation error",
			err:     context.Canceled,
			outcome: types.LLMAuditOutcomeCanceled,
			wantErr: context.Canceled.Error(),
		},
		{
			name:    "actual proxy error",
			err:     errors.New("proxy failed"),
			outcome: types.LLMAuditOutcomeError,
			wantErr: "proxy failed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &llmAuditRecorder{}
			recorder.setOutcome(tt.err)
			if recorder.log.Outcome != tt.outcome {
				t.Fatalf("expected outcome %q, got %q", tt.outcome, recorder.log.Outcome)
			}
			if recorder.log.Error != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, recorder.log.Error)
			}
		})
	}
}

func TestParseLLMClientUserAgent(t *testing.T) {
	for _, tt := range []struct {
		userAgent string
		client    string
		version   string
	}{
		{userAgent: "claude-code/2.1.176", client: llmAuditClientClaudeCode, version: "2.1.176"},
		{userAgent: "claude-cli/2.1.176 (external, cli)", client: llmAuditClientClaudeCode, version: "2.1.176"},
		{userAgent: "codex_cli_rs/0.142.4 (Mac OS 26.5.1; arm64) ghostty/1.3.1", client: llmAuditClientCodex, version: "0.142.4"},
		{userAgent: "codex-tui/0.142.4 (Mac OS 26.5.1; arm64) ghostty/1.3.1 (codex-tui; 0.142.4)", client: llmAuditClientCodex, version: "0.142.4"},
		{userAgent: "other-client/1.2.3", client: "other-client", version: "1.2.3"},
		{userAgent: "opencode/brew/1.2.3", client: "opencode", version: "brew/1.2.3"},
		{userAgent: "", client: "", version: ""},
		{userAgent: "unknown-client", client: "unknown-client", version: ""},
	} {
		client, version := parseLLMClientUserAgent(tt.userAgent)
		if client != tt.client || version != tt.version {
			t.Fatalf("parseLLMClientUserAgent(%q) = %q/%q, want %q/%q", tt.userAgent, client, version, tt.client, tt.version)
		}
	}
}

func TestExtractLLMClientSessionID(t *testing.T) {
	for _, tt := range []struct {
		name          string
		modelProvider string
		body          string
		want          string
	}{
		{
			name:          "openai client metadata session id",
			modelProvider: system.OpenAIModelProvider,
			body:          `{"client_metadata":{"session_id":"openai-session"}}`,
			want:          "openai-session",
		},
		{
			name:          "openai ignores codex metadata fallback",
			modelProvider: system.OpenAIModelProvider,
			body:          `{"client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"ignored\"}"}}`,
		},
		{
			name:          "anthropic metadata user id session id",
			modelProvider: system.AnthropicModelProvider,
			body:          `{"metadata":{"user_id":"{\"session_id\":\"claude-session\"}"}}`,
			want:          "claude-session",
		},
		{
			name:          "anthropic malformed metadata user id",
			modelProvider: system.AnthropicModelProvider,
			body:          `{"metadata":{"user_id":"not-json"}}`,
		},
		{
			name:          "wrong provider",
			modelProvider: "other",
			body:          `{"client_metadata":{"session_id":"ignored"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLLMClientSessionID(tt.modelProvider, []byte(tt.body)); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtractLLMReasoningEffort(t *testing.T) {
	for _, tt := range []struct {
		name          string
		modelProvider string
		body          string
		want          string
	}{
		{
			name:          "openai reasoning effort",
			modelProvider: system.OpenAIModelProvider,
			body:          `{"reasoning":{"effort":"medium"}}`,
			want:          "medium",
		},
		{
			name:          "anthropic output effort",
			modelProvider: system.AnthropicModelProvider,
			body:          `{"output_config":{"effort":"high"}}`,
			want:          "high",
		},
		{
			name:          "anthropic thinking type is not effort",
			modelProvider: system.AnthropicModelProvider,
			body:          `{"thinking":{"type":"adaptive"}}`,
		},
		{
			name:          "wrong provider",
			modelProvider: "other",
			body:          `{"reasoning":{"effort":"medium"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLLMReasoningEffort(tt.modelProvider, []byte(tt.body)); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestLLMResponseAccumulatorOpenAITerminalResponseWins(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_123","model":"gpt-5","output":null}}`,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"ignored"}`,
		`data: {"type":"response.completed","response":{"id":"resp_123","model":"gpt-5","status":"completed","output":[{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"final"}]}]}}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.Get(got, "output.0.content.0.text").String() != "final" {
		t.Fatalf("expected terminal response text, got %s", got)
	}
	if a.ResponseID() != "resp_123" {
		t.Fatalf("expected response ID, got %q", a.ResponseID())
	}
}

func TestLLMResponseAccumulatorOpenAIPartialTextAndSplitLine(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_partial","model":"gpt-5","output":null}}` + "\n"))
	a.Write([]byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hel`))
	a.Write([]byte(`lo"}` + "\n"))

	got := a.JSON()
	if gjson.Get(got, "output.0.content.0.text").String() != "hello" {
		t.Fatalf("expected accumulated text, got %s", got)
	}
}

func TestLLMResponseAccumulatorOpenAIFunctionAndReasoning(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_tools","model":"gpt-5","output":[]}}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"id":"call_1","type":"function_call","name":"lookup","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"q\":"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"\"hi\"}"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"id":"rs_1","type":"reasoning","encrypted_content":"secret"}}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.Get(got, "output.0.arguments").String() != `{"q":"hi"}` {
		t.Fatalf("expected function arguments, got %s", got)
	}
	if gjson.Get(got, "output.1.encrypted_content").String() != "secret" {
		t.Fatalf("expected reasoning encrypted content, got %s", got)
	}
}

func TestLLMResponseAccumulatorAnthropicMessage(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.AnthropicModelProvider)
	a.Write([]byte(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_123","type":"message","role":"assistant","model":"claude","content":[],"usage":{"input_tokens":1}}}`,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.Get(got, "content.0.text").String() != "hello" {
		t.Fatalf("expected anthropic text, got %s", got)
	}
	if gjson.Get(got, "usage.output_tokens").Int() != 5 {
		t.Fatalf("expected usage, got %s", got)
	}
	if a.ResponseID() != "msg_123" {
		t.Fatalf("expected message ID, got %q", a.ResponseID())
	}
}

func TestLLMResponseAccumulatorAnthropicToolAndThinking(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.AnthropicModelProvider)
	a.Write([]byte(strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_tool","type":"message","role":"assistant","content":[]}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"lookup","input":{}}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"hi\"}"}}`,
		`data: {"type":"content_block_stop","index":0}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"ponder"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"sig"}}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.Get(got, "content.0.input.q").String() != "hi" {
		t.Fatalf("expected tool input, got %s", got)
	}
	if gjson.Get(got, "content.1.thinking").String() != "ponder" || gjson.Get(got, "content.1.signature").String() != "sig" {
		t.Fatalf("expected thinking/signature, got %s", got)
	}
}

func TestLLMResponseAccumulatorNonStreamAndEmpty(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(`{"id":"resp_plain","status":"completed"}`))
	if got := a.JSON(); gjson.Get(got, "id").String() != "resp_plain" {
		t.Fatalf("expected plain JSON response, got %s", got)
	}

	empty := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	if got := empty.JSON(); got != "{}" {
		t.Fatalf("expected empty object, got %s", got)
	}
}

func TestLLMAuditRecorderStoresAggregatedResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	recorder := newLLMAuditRecorder(req, nil, 5<<20)
	recorder.setModel(system.OpenAIModelProvider, "", "")
	chunk := []byte(`data: {"type":"response.created","response":{"id":"resp_rec","output":[]}}` + "\n")
	recorder.captureResponseChunk(chunk)
	accumulator := gatewayllmaudit.NewResponseAccumulator(recorder.log.ModelProvider)
	accumulator.Write(recorder.responseStream.Bytes())
	recorder.log.ResponseBody = accumulator.JSON()

	if gjson.Get(recorder.log.ResponseBody, "id").String() != "resp_rec" {
		t.Fatalf("expected aggregated response body, got %s", recorder.log.ResponseBody)
	}
}

func TestLLMAuditRecorderCapsResponseCapture(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	recorder := newLLMAuditRecorder(req, nil, 5)
	recorder.captureResponseChunk([]byte("hello"))
	recorder.captureResponseChunk([]byte(" world"))

	if got := recorder.responseStream.String(); got != "hello" {
		t.Fatalf("expected capped response stream, got %q", got)
	}
}
