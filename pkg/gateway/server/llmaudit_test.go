package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestRedactedHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret"},
		"X-Api-Key":     []string{"secret"},
		"Content-Type":  []string{"application/json"},
	}

	got := redactedHeaders(headers)
	if strings.Contains(got, "Bearer secret") || strings.Contains(got, "secret") {
		t.Fatalf("expected secrets to be redacted, got %s", got)
	}
	if !strings.Contains(got, "application/json") {
		t.Fatalf("expected non-sensitive header to remain, got %s", got)
	}
}

func TestExtractLLMResponseText(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"hello "}}]}`,
		`data: {"type":"response.output_text.delta","delta":"world"}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"!"}}`,
	}, "\n")

	if got := extractLLMResponseText([]byte(body)); got != "hello world!" {
		t.Fatalf("expected response text, got %q", got)
	}
}
