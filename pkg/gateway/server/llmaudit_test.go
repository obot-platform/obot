package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	gatewayllmaudit "github.com/obot-platform/obot/pkg/gateway/llmaudit"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
)

func TestRedactedHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization":                               []string{"Bearer secret"},
		"X-Api-Key":                                   []string{"secret"},
		"Content-Type":                                []string{"application/json"},
		"X-Ratelimit-Limit-Tokens":                    []string{"x-limit"},
		"X-Ratelimit-Remaining-Tokens":                []string{"x-remaining"},
		"X-Ratelimit-Reset-Tokens":                    []string{"x-reset"},
		"Anthropic-Ratelimit-Input-Tokens-Limit":      []string{"input-limit"},
		"Anthropic-Ratelimit-Input-Tokens-Remaining":  []string{"input-remaining"},
		"Anthropic-Ratelimit-Input-Tokens-Reset":      []string{"input-reset"},
		"Anthropic-Ratelimit-Output-Tokens-Limit":     []string{"output-limit"},
		"Anthropic-Ratelimit-Output-Tokens-Remaining": []string{"output-remaining"},
		"Anthropic-Ratelimit-Output-Tokens-Reset":     []string{"output-reset"},
		"Anthropic-Ratelimit-Requests-Limit":          []string{"requests-limit"},
		"Anthropic-Ratelimit-Requests-Remaining":      []string{"requests-remaining"},
		"Anthropic-Ratelimit-Requests-Reset":          []string{"requests-reset"},
		"Anthropic-Ratelimit-Tokens-Limit":            []string{"tokens-limit"},
		"Anthropic-Ratelimit-Tokens-Remaining":        []string{"tokens-remaining"},
		"Anthropic-Ratelimit-Tokens-Reset":            []string{"tokens-reset"},
		"Anthropic-Ratelimit-Api-Key":                 []string{"sensitive-lookalike"},
	}

	got := redactedHeaders(headers)
	if strings.Contains(string(got), "Bearer secret") || strings.Contains(string(got), "secret") {
		t.Fatalf("expected secrets to be redacted, got %s", got)
	}
	if !strings.Contains(string(got), "application/json") {
		t.Fatalf("expected non-sensitive header to remain, got %s", got)
	}
	for _, value := range []string{
		"x-limit", "x-remaining", "x-reset",
		"input-limit", "input-remaining", "input-reset",
		"output-limit", "output-remaining", "output-reset",
		"requests-limit", "requests-remaining", "requests-reset",
		"tokens-limit", "tokens-remaining", "tokens-reset",
	} {
		if !strings.Contains(string(got), value) {
			t.Fatalf("expected rate-limit header value %q to remain, got %s", value, got)
		}
	}
	if strings.Contains(string(got), "sensitive-lookalike") {
		t.Fatalf("expected sensitive rate-limit lookalike to be redacted, got %s", got)
	}
	if shouldRedactHeader("ANTHROPIC-RATELIMIT-TOKENS-LIMIT") {
		t.Fatal("expected rate-limit header matching to be case-insensitive")
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

func TestLLMAuditRecorderCapturesOriginalAndPolicyModifiedRequestBodies(t *testing.T) {
	recorder := &llmAuditRecorder{}
	recorder.setRequestBody([]byte(`{"prompt":"original"}`))
	recorder.setPolicyModifiedRequestBody([]byte(`{"prompt":"blocked"}`))

	if got := string(recorder.log.RequestBody); got != `{"prompt":"original"}` {
		t.Fatalf("expected original request body, got %q", got)
	}
	if got := string(recorder.log.PolicyModifiedRequestBody); got != `{"prompt":"blocked"}` {
		t.Fatalf("expected policy-modified request body, got %q", got)
	}
	if !recorder.log.MessagePolicyTriggered {
		t.Fatal("expected input policy trigger metadata")
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
		{userAgent: "gRi/JS 0.94.0", client: llmAuditClientClaudeCode, version: ""},
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
		name    string
		dialect nanobottypes.Dialect
		headers http.Header
		body    string
		want    string
	}{
		{
			name:    "Claude Code session header preferred over body",
			dialect: nanobottypes.DialectAnthropicMessages,
			headers: http.Header{claudeCodeSessionIDHeader: []string{"header-session"}},
			body:    `{"metadata":{"user_id":"{\"session_id\":\"body-session\"}"}}`,
			want:    "header-session",
		},
		{
			name:    "OpenAI dialect client metadata session id",
			dialect: nanobottypes.DialectOpenAIResponses,
			body:    `{"client_metadata":{"session_id":"openai-session"}}`,
			want:    "openai-session",
		},
		{
			name:    "OpenAI dialect ignores Codex metadata fallback",
			dialect: nanobottypes.DialectOpenAIResponses,
			body:    `{"client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"ignored\"}"}}`,
		},
		{
			name:    "Anthropic dialect metadata user id session id",
			dialect: nanobottypes.DialectAnthropicMessages,
			body:    `{"metadata":{"user_id":"{\"session_id\":\"claude-session\"}"}}`,
			want:    "claude-session",
		},
		{
			name:    "Anthropic dialect malformed metadata user id",
			dialect: nanobottypes.DialectAnthropicMessages,
			body:    `{"metadata":{"user_id":"not-json"}}`,
		},
		{
			name: "unknown dialect",
			body: `{"client_metadata":{"session_id":"ignored"}}`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLLMClientSessionID(tt.dialect, tt.headers, []byte(tt.body)); got != tt.want {
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
	if gjson.GetBytes(got, "output.0.content.0.text").String() != "final" {
		t.Fatalf("expected terminal response text, got %s", got)
	}
	if a.ResponseID() != "resp_123" {
		t.Fatalf("expected response ID, got %q", a.ResponseID())
	}
}

func TestLLMResponseAccumulatorBedrockOpenAI(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.AmazonBedrockAPIKeyModelProvider, "/api/llm-proxy/aws-bedrock-api-key/openai/v1/responses")
	a.Write([]byte(strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_bedrock","model":"openai.gpt-5.5","output":[]}}`,
		`data: {"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"hello"}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.GetBytes(got, "output.0.content.0.text").String() != "hello" {
		t.Fatalf("expected Bedrock OpenAI text, got %s", got)
	}
	if a.ResponseID() != "resp_bedrock" {
		t.Fatalf("expected response ID, got %q", a.ResponseID())
	}
}

func TestLLMResponseAccumulatorOpenAIPartialTextAndSplitLine(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(`data: {"type":"response.created","response":{"id":"resp_partial","model":"gpt-5","output":null}}` + "\n"))
	a.Write([]byte(`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"hel`))
	a.Write([]byte(`lo"}` + "\n"))

	got := a.JSON()
	if gjson.GetBytes(got, "output.0.content.0.text").String() != "hello" {
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
	if gjson.GetBytes(got, "output.0.arguments").String() != `{"q":"hi"}` {
		t.Fatalf("expected function arguments, got %s", got)
	}
	if gjson.GetBytes(got, "output.1.encrypted_content").String() != "secret" {
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
	if gjson.GetBytes(got, "content.0.text").String() != "hello" {
		t.Fatalf("expected anthropic text, got %s", got)
	}
	if gjson.GetBytes(got, "usage.output_tokens").Int() != 5 {
		t.Fatalf("expected usage, got %s", got)
	}
	if a.ResponseID() != "msg_123" {
		t.Fatalf("expected message ID, got %q", a.ResponseID())
	}
}

func TestLLMResponseAccumulatorBedrockAnthropic(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.AmazonBedrockModelProvider, "/api/llm-proxy/aws-bedrock/anthropic/v1/messages")
	a.Write([]byte(strings.Join([]string{
		`data: {"type":"message_start","message":{"id":"msg_bedrock","type":"message","role":"assistant","model":"claude","content":[]}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
	}, "\n") + "\n"))

	got := a.JSON()
	if gjson.GetBytes(got, "content.0.text").String() != "hello" {
		t.Fatalf("expected Bedrock Anthropic text, got %s", got)
	}
	if a.ResponseID() != "msg_bedrock" {
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
	if gjson.GetBytes(got, "content.0.input.q").String() != "hi" {
		t.Fatalf("expected tool input, got %s", got)
	}
	if gjson.GetBytes(got, "content.1.thinking").String() != "ponder" || gjson.GetBytes(got, "content.1.signature").String() != "sig" {
		t.Fatalf("expected thinking/signature, got %s", got)
	}
}

func TestLLMResponseAccumulatorNonStreamAndEmpty(t *testing.T) {
	a := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	a.Write([]byte(`{"id":"resp_plain","status":"completed"}`))
	if got := a.JSON(); gjson.GetBytes(got, "id").String() != "resp_plain" {
		t.Fatalf("expected plain JSON response, got %s", got)
	}

	empty := gatewayllmaudit.NewResponseAccumulator(system.OpenAIModelProvider)
	if got := empty.JSON(); !bytes.Equal(got, []byte(`{}`)) {
		t.Fatalf("expected empty object, got %s", got)
	}
}

func TestLLMAuditRecorderStoresAggregatedResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	recorder := newLLMAuditRecorder(req, nil, 5<<20)
	recorder.setModel(system.OpenAIModelProvider, "", "")
	chunk := []byte(`data: {"type":"response.created","response":{"id":"resp_rec","output":[]}}` + "\n")
	recorder.captureResponseChunk(chunk)
	accumulator := gatewayllmaudit.NewResponseAccumulator(recorder.log.ModelProvider, recorder.log.RequestPath)
	accumulator.Write(recorder.responseStream.Bytes())
	recorder.log.ResponseBody = accumulator.JSON()

	if gjson.GetBytes(recorder.log.ResponseBody, "id").String() != "resp_rec" {
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
