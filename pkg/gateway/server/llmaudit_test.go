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
