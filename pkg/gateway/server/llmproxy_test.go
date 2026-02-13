package server

import (
	"bufio"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestModifyResponse_PathFiltering(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		statusCode  int
		wantWrapped bool
	}{
		{"chat completions", "/v1/chat/completions", http.StatusOK, true},
		{"anthropic messages", "/v1/messages", http.StatusOK, true},
		{"openai responses", "/v1/responses", http.StatusOK, true},
		{"unknown path", "/v1/embeddings", http.StatusOK, false},
		{"non-200 status", "/v1/chat/completions", http.StatusBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &responseModifier{}
			body := io.NopCloser(strings.NewReader("{}"))
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
				Request:    &http.Request{URL: &url.URL{Path: tt.path}},
			}

			if err := r.modifyResponse(resp); err != nil {
				t.Fatal(err)
			}

			// If wrapped, resp.Body should be the responseModifier itself
			if tt.wantWrapped && resp.Body != r {
				t.Error("expected response body to be wrapped by responseModifier")
			}
			if !tt.wantWrapped && resp.Body != body {
				t.Error("expected response body to remain unwrapped")
			}
		})
	}
}

func TestResponseModifier_OpenAIChatCompletions(t *testing.T) {
	// Streaming chat completions format
	stream := "data: {\"model\":\"gpt-4o\",\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":20,\"total_tokens\":30}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 10 {
		t.Errorf("promptTokens = %d, want 10", r.promptTokens)
	}
	if r.completionTokens != 20 {
		t.Errorf("completionTokens = %d, want 20", r.completionTokens)
	}
	if r.totalTokens != 30 {
		t.Errorf("totalTokens = %d, want 30", r.totalTokens)
	}
	if r.model != "gpt-4o" {
		t.Errorf("model = %q, want %q", r.model, "gpt-4o")
	}
}

func TestResponseModifier_AnthropicMessages(t *testing.T) {
	// Anthropic streaming: message_start has usage under "message", message_delta has top-level usage
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	// Read message_start
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	// Read message_delta
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 25 {
		t.Errorf("promptTokens = %d, want 25", r.promptTokens)
	}
	// message_delta output_tokens is cumulative (15 total), not incremental,
	// so it supersedes the initial output_tokens (1) from message_start.
	if r.completionTokens != 15 {
		t.Errorf("completionTokens = %d, want 15 (cumulative from message_delta)", r.completionTokens)
	}
	if r.model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want %q", r.model, "claude-sonnet-4-20250514")
	}
}

func TestResponseModifier_OpenAIResponsesAPI(t *testing.T) {
	// Responses API streaming: response.completed has usage nested under "response"
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-4o\",\"usage\":{\"input_tokens\":50,\"output_tokens\":100,\"total_tokens\":150}}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 50 {
		t.Errorf("promptTokens = %d, want 50", r.promptTokens)
	}
	if r.completionTokens != 100 {
		t.Errorf("completionTokens = %d, want 100", r.completionTokens)
	}
	if r.totalTokens != 150 {
		t.Errorf("totalTokens = %d, want 150", r.totalTokens)
	}
	if r.model != "gpt-4o" {
		t.Errorf("model = %q, want %q", r.model, "gpt-4o")
	}
}

func TestResponseModifier_NonStreamingResponse(t *testing.T) {
	// Non-streaming: plain JSON body with top-level usage
	body := "{\"model\":\"gpt-4o\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":10,\"total_tokens\":15}}\n"

	r := &responseModifier{
		stream: false,
		b:      bufio.NewReader(strings.NewReader(body)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 5 {
		t.Errorf("promptTokens = %d, want 5", r.promptTokens)
	}
	if r.completionTokens != 10 {
		t.Errorf("completionTokens = %d, want 10", r.completionTokens)
	}
	if r.totalTokens != 15 {
		t.Errorf("totalTokens = %d, want 15", r.totalTokens)
	}
	if r.model != "gpt-4o" {
		t.Errorf("model = %q, want %q", r.model, "gpt-4o")
	}
}

func TestResponseModifier_ModelFromRequestPreserved(t *testing.T) {
	// If model is already set from the request, don't overwrite from response
	stream := "data: {\"model\":\"gpt-4o-realmodel\",\"usage\":{\"prompt_tokens\":1}}\n"

	r := &responseModifier{
		stream: true,
		model:  "my-alias",
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.model != "my-alias" {
		t.Errorf("model = %q, want %q (should preserve original)", r.model, "my-alias")
	}
}

func TestResponseModifier_AnthropicCumulativeTokens(t *testing.T) {
	// Anthropic message_delta reports cumulative tokens that supersede earlier counts.
	// This mirrors the web search case where message_delta has higher input_tokens.
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-opus-4-6\",\"usage\":{\"input_tokens\":2679,\"output_tokens\":3}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":10682,\"output_tokens\":510}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	if r.promptTokens != 10682 {
		t.Errorf("promptTokens = %d, want 10682 (cumulative from message_delta)", r.promptTokens)
	}
	if r.completionTokens != 510 {
		t.Errorf("completionTokens = %d, want 510 (cumulative from message_delta)", r.completionTokens)
	}
	// totalTokens should be 0 since Anthropic doesn't provide it explicitly;
	// it gets derived in Close().
	if r.totalTokens != 0 {
		t.Errorf("totalTokens = %d, want 0 (derived at Close time)", r.totalTokens)
	}
}

func TestResponseModifier_TotalTokensDerivedAtClose(t *testing.T) {
	// When no explicit total_tokens is provided (e.g. Anthropic), it should
	// be derived from prompt + completion at Close time.
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	// Simulate Close() logic without needing a real DB client.
	totalTokens := r.totalTokens
	if totalTokens == 0 {
		totalTokens = r.promptTokens + r.completionTokens
	}
	if totalTokens != 40 {
		t.Errorf("derived totalTokens = %d, want 40 (25 prompt + 15 completion)", totalTokens)
	}
}

func TestResponseModifier_StreamNonDataLinesPassThrough(t *testing.T) {
	// Non-data lines (like "event: ..." lines) should pass through without parsing
	stream := "event: message_start\n"

	r := &responseModifier{
		stream: true,
		b:      bufio.NewReader(strings.NewReader(stream)),
		c:      io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf[:n]) != "event: message_start\n" {
		t.Errorf("got %q, want %q", string(buf[:n]), "event: message_start\n")
	}
	if r.promptTokens != 0 || r.completionTokens != 0 {
		t.Error("non-data lines should not affect token counts")
	}
}

func TestExtractModelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			"top-level model (OpenAI/Anthropic request)",
			`{"model":"gpt-4o","messages":[]}`,
			"gpt-4o",
		},
		{
			"nested under message",
			`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`,
			"claude-sonnet-4-20250514",
		},
		{
			"nested under response",
			`{"type":"response.completed","response":{"model":"gpt-4o"}}`,
			"gpt-4o",
		},
		{
			"top-level takes precedence over nested",
			`{"model":"top-level","message":{"model":"nested"}}`,
			"top-level",
		},
		{
			"empty body",
			`{}`,
			"",
		},
		{
			"no model anywhere",
			`{"messages":[{"role":"user","content":"hello"}]}`,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModelFromBody([]byte(tt.body))
			if got != tt.want {
				t.Errorf("extractModelFromBody() = %q, want %q", got, tt.want)
			}
		})
	}
}
