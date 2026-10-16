package server

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/tidwall/gjson"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestNewRunTokenActivityCapturesUserAndAPIKey(t *testing.T) {
	requestUser := &user.DefaultInfo{
		UID: "7",
		Extra: map[string][]string{
			principal.APIKeyIDExtra:   {"42"},
			principal.APIKeyNameExtra: {"CLI token"},
		},
	}
	usage := types.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalSpend: 0.25}

	activity := newRunTokenActivity(requestUser, "gpt-5", usage)

	if activity.UserID != "7" || activity.Model != "gpt-5" || activity.Usage != usage {
		t.Fatalf("activity identity or usage = %#v", activity)
	}
	if activity.APIKeyID == nil || *activity.APIKeyID != 42 || activity.APIKeyName != "CLI token" {
		t.Fatalf("API key attribution = ID %v, name %q; want ID 42, name %q", activity.APIKeyID, activity.APIKeyName, "CLI token")
	}
}

func TestNewRunTokenActivityLeavesDirectUserUnattributed(t *testing.T) {
	activity := newRunTokenActivity(&user.DefaultInfo{UID: "7"}, "gpt-5", types.TokenUsage{})
	if activity.APIKeyID != nil || activity.APIKeyName != "" {
		t.Fatalf("direct user gained API key attribution: %#v", activity)
	}
}

func TestModifyResponse_WrapGate(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		statusCode  int
		wantWrapped bool
	}{
		{
			name:        "anthropic messages",
			path:        "/v1/messages",
			statusCode:  http.StatusOK,
			wantWrapped: true,
		},
		{
			name:        "openai responses",
			path:        "/v1/responses",
			statusCode:  http.StatusOK,
			wantWrapped: true,
		},
		{
			name:        "unknown path",
			path:        "/v1/embeddings",
			statusCode:  http.StatusOK,
			wantWrapped: false,
		},
		{
			name:        "non-200 status",
			path:        "/v1/messages",
			statusCode:  http.StatusBadRequest,
			wantWrapped: false,
		},
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

func TestResponseModifier_AnthropicMessages(t *testing.T) {
	// Anthropic streaming: message_start has usage under "message", message_delta has top-level usage
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream:            true,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &messageTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(stream)),
		c:                 io.NopCloser(strings.NewReader("")),
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

	got := r.tokenUsageTracker.getTokenUsage()
	if got.InputTokens != 25 {
		t.Errorf("InputTokens = %d, want 25", got.InputTokens)
	}
	// message_delta output_tokens is cumulative (15 total), not incremental,
	// so it supersedes the initial output_tokens (1) from message_start.
	if got.OutputTokens != 15 {
		t.Errorf("OutputTokens = %d, want 15 (cumulative from message_delta)", got.OutputTokens)
	}
}

func TestResponseModifier_OpenAIResponsesAPI(t *testing.T) {
	// Responses API streaming: response.completed has usage nested under "response"
	stream := "data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-4o\",\"usage\":{\"input_tokens\":50,\"output_tokens\":100,\"total_tokens\":150}}}\n"

	r := &responseModifier{
		stream:            true,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &responseTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(stream)),
		c:                 io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	got := r.tokenUsageTracker.getTokenUsage()
	if got.InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", got.InputTokens)
	}
	if got.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", got.OutputTokens)
	}
	if total := got.TotalTokens; total != 150 {
		t.Errorf("totalTokens() = %d, want 150", total)
	}
}

func TestResponseModifier_NonStreamingPrettyPrintedResponseTracksUsage(t *testing.T) {
	tracker := newTokenUsageTracker(v1.Model{
		Spec: v1.ModelSpec{Manifest: types2.ModelManifest{
			Dialect: string(llmtypes.DialectOpenAIResponses),
		}},
	})
	recorder := newLLMAuditRecorder(
		httptest.NewRequest(http.MethodPost, "/v1/responses", nil),
		nil,
		5<<20,
	)
	body := `{
  "id": "resp_1",
  "object": "response",
  "status": "completed",
  "model": "gpt-5.4-2026-03-05",
  "usage": {
    "input_tokens": 7,
    "input_tokens_details": {
      "cache_write_tokens": 0,
      "cached_tokens": 0
    },
    "output_tokens": 11,
    "output_tokens_details": {
      "reasoning_tokens": 0
    },
    "total_tokens": 18
  }
}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    httptest.NewRequest(http.MethodPost, "/v1/responses", nil),
	}
	r := &responseModifier{
		tokenUsageTracker: tracker,
		audit:             recorder,
	}

	if err := r.modifyResponse(resp); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}

	if got := recorder.responseStream.String(); got != body {
		t.Fatalf("audit response = %q, want %q", got, body)
	}
	usage := tracker.getTokenUsage()
	if usage.InputTokens != 7 || usage.OutputTokens != 11 || usage.TotalTokens != 18 {
		t.Fatalf("token usage = %+v, want 7 input, 11 output, 18 total", usage)
	}
}

func TestResponseModifier_NonStreamingResponse(t *testing.T) {
	body := "{\"model\":\"gpt-4o\",\"usage\":{\"input_tokens\":5,\"output_tokens\":10,\"total_tokens\":15}}\n"

	r := &responseModifier{
		stream:            false,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &responseTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(body)),
		c:                 io.NopCloser(strings.NewReader("")),
	}

	if _, err := io.ReadAll(r); err != nil {
		t.Fatal(err)
	}

	got := r.tokenUsageTracker.getTokenUsage()
	if got.InputTokens != 5 {
		t.Errorf("InputTokens = %d, want 5", got.InputTokens)
	}
	if got.OutputTokens != 10 {
		t.Errorf("OutputTokens = %d, want 10", got.OutputTokens)
	}
	if total := got.TotalTokens; total != 15 {
		t.Errorf("totalTokens() = %d, want 15", total)
	}
}

func TestResponseModifierCapturesAuditResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	recorder := newLLMAuditRecorder(req, nil, 5<<20)
	body := "{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n"
	r := &responseModifier{
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &responseTokenUsageTracker{}},
		audit:             recorder,
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}

	if err := r.modifyResponse(resp); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}

	if got := recorder.responseStream.String(); got != body {
		t.Fatalf("captured response body = %q, want %q", got, body)
	}
}

func TestResponseModifierPreservesUpstreamErrorBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	recorder := newLLMAuditRecorder(req, nil, 5<<20)
	body := `{"error":{"code":"validation_error","message":"model does not support this API"}}`
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}

	if err := (&responseModifier{audit: recorder}).modifyResponse(resp); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("response body = %q, want %q", got, body)
	}
	if got := recorder.responseStream.String(); got != body {
		t.Fatalf("captured response body = %q, want %q", got, body)
	}
	if got := recorder.log.ResponseStatus; got != http.StatusBadRequest {
		t.Fatalf("response status = %d, want %d", got, http.StatusBadRequest)
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
		stream:            true,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &messageTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(stream)),
		c:                 io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	got := r.tokenUsageTracker.getTokenUsage()
	if got.InputTokens != 10682 {
		t.Errorf("InputTokens = %d, want 10682 (cumulative from message_delta)", got.InputTokens)
	}
	if got.OutputTokens != 510 {
		t.Errorf("OutputTokens = %d, want 510 (cumulative from message_delta)", got.OutputTokens)
	}
	if total := got.TotalTokens; total != 10682+510 {
		t.Errorf("totalTokens() = %d, want %d (derived)", total, 10682+510)
	}
}

func TestResponseModifier_TotalTokensDerivedAtClose(t *testing.T) {
	stream := "data: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-sonnet-4-20250514\",\"usage\":{\"input_tokens\":25,\"output_tokens\":1}}}\n" +
		"data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":15}}\n"

	r := &responseModifier{
		stream:            true,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &messageTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(stream)),
		c:                 io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(buf); err != nil {
		t.Fatal(err)
	}

	// Simulate Close() logic without needing a real DB client.
	if total := r.tokenUsageTracker.getTokenUsage().TotalTokens; total != 40 {
		t.Errorf("derived totalTokens = %d, want 40 (25 input + 15 output)", total)
	}
}

func TestResponseModifier_StreamNonDataLinesPassThrough(t *testing.T) {
	// Non-data lines (like "event: ..." lines) should pass through without parsing
	stream := "event: message_start\n"

	r := &responseModifier{
		stream:            true,
		tokenUsageTracker: &threadSafeTokenUsageTracker{inner: &messageTokenUsageTracker{}},
		b:                 bufio.NewReader(strings.NewReader(stream)),
		c:                 io.NopCloser(strings.NewReader("")),
	}

	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if string(buf[:n]) != "event: message_start\n" {
		t.Errorf("got %q, want %q", string(buf[:n]), "event: message_start\n")
	}
	if got := r.tokenUsageTracker.getTokenUsage(); got.InputTokens != 0 || got.OutputTokens != 0 {
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
			name: "top-level model (OpenAI/Anthropic request)",
			body: `{"model":"gpt-4o","messages":[]}`,
			want: "gpt-4o",
		},
		{
			name: "nested under message",
			body: `{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`,
			want: "claude-sonnet-4-20250514",
		},
		{
			name: "nested under response",
			body: `{"type":"response.completed","response":{"model":"gpt-4o"}}`,
			want: "gpt-4o",
		},
		{
			name: "top-level takes precedence over nested",
			body: `{"model":"top-level","message":{"model":"nested"}}`,
			want: "top-level",
		},
		{
			name: "empty body",
			body: `{}`,
			want: "",
		},
		{
			name: "no model anywhere",
			body: `{"messages":[{"role":"user","content":"hello"}]}`,
			want: "",
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

func TestRewriteModelInBody(t *testing.T) {
	body := `{"model":"anthropic-model-provider/anthropic-claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`

	rewritten, err := rewriteModelInBody([]byte(body), "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := extractModelFromBody(rewritten); got != "claude-sonnet-4-6" {
		t.Fatalf("model = %q, want claude-sonnet-4-6", got)
	}
}

func TestLLMRewriteRequest_RemovesAcceptEncoding(t *testing.T) {
	u := mustParseURL("https://api.example.com/v1")

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/v1/responses", nil)
	req.SetPathValue("path", "responses")
	req.Header.Set("Accept-Encoding", "gzip")

	proxyReq := &httputil.ProxyRequest{In: req, Out: req.Clone(req.Context())}
	llmRewriteRequest(*u)(proxyReq)
	req = proxyReq.Out

	if got := req.Header.Get("Accept-Encoding"); got != "" {
		t.Fatalf("Accept-Encoding = %q, want empty", got)
	}
}

// TestLLMRewriteRequest_UpstreamPath asserts the upstream URL.Path produced by
// llmRewriteRequest for every (base URL, reqPath) combination the proxy
// should support. Every reqPath is grounded in real source — either nanobot
// (nanobot/pkg/llm/{anthropic,responses}/client.go) or the
// official SDK each documented external coding tool uses.
//
// The expected paths are also what modifyResponse in llmproxy.go checks against
// (/v1/messages and /v1/responses) for token counting and policy enforcement.
func TestLLMRewriteRequest_UpstreamPath(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		reqPath string
		want    string
	}{
		// --- Nanobot dialects (exact suffixes from nanobot source) ---
		// nanobot/pkg/llm/anthropic/client.go → BaseURL + "/messages"
		{
			name:    "nanobot AnthropicMessages dialect",
			baseURL: "https://api.anthropic.com/v1",
			reqPath: "messages",
			want:    "/v1/messages",
		},
		// nanobot/pkg/llm/responses/client.go → BaseURL + "/responses"
		{
			name:    "nanobot OpenAIResponses dialect",
			baseURL: "https://api.openai.com/v1",
			reqPath: "responses",
			want:    "/v1/responses",
		},
		// nanobot/pkg/llm/client.go: OpenResponses uses the responses client
		{
			name:    "nanobot OpenResponses dialect",
			baseURL: "http://127.0.0.1:8080",
			reqPath: "responses",
			want:    "/v1/responses",
		},

		// --- External coding tools pointed at the passthrough routes ---
		// Claude Code uses the official Anthropic SDK; its default base URL is
		// "https://api.anthropic.com" (no /v1) and the SDK appends /v1/messages.
		// With base=…/api/llm-proxy/anthropic, the mux captures "v1/messages".
		{
			name:    "Claude Code → /api/llm-proxy/anthropic",
			baseURL: "https://api.anthropic.com/v1",
			reqPath: "v1/messages",
			want:    "/v1/messages",
		},
		// OpenCode's Anthropic provider (Vercel AI SDK) is documented with base
		// "https://api.anthropic.com/v1" and appends "/messages". Pointed at
		// base=…/api/llm-proxy/anthropic/v1 the mux still captures "v1/messages".
		{
			name:    "OpenCode (Anthropic via Vercel AI SDK) → /api/llm-proxy/anthropic/v1",
			baseURL: "https://api.anthropic.com/v1",
			reqPath: "v1/messages",
			want:    "/v1/messages",
		},
		// OpenAI Python/TS SDKs default to base="https://api.openai.com/v1" and
		// append "/responses". Pointed at base=…/api/llm-proxy/openai/v1 → mux
		// captures "v1/responses".
		{
			name:    "OpenAI SDK (Responses API) → /api/llm-proxy/openai/v1",
			baseURL: "https://api.openai.com/v1",
			reqPath: "v1/responses",
			want:    "/v1/responses",
		},
		{
			name:    "OpenAI SDK (Responses API) → /api/llm-proxy/generic-responses/v1",
			baseURL: "https://models.example/v1",
			reqPath: "v1/responses",
			want:    "/v1/responses",
		},
		// Same SDK, chat completions endpoint.
		{
			name:    "OpenAI SDK (Chat Completions) → /api/llm-proxy/openai/v1",
			baseURL: "https://api.openai.com/v1",
			reqPath: "v1/chat/completions",
			want:    "/v1/chat/completions",
		},
		// OpenAI SDK list-models endpoint (GET /v1/models).
		{
			name:    "OpenAI SDK (list models) → /api/llm-proxy/openai/v1",
			baseURL: "https://api.openai.com/v1",
			reqPath: "v1/models",
			want:    "/v1/models",
		},
		{
			name:    "Claude Code Mantle → /api/llm-proxy/aws-bedrock/anthropic",
			baseURL: "https://bedrock-mantle.us-east-1.api.aws/anthropic/v1",
			reqPath: "v1/messages",
			want:    "/anthropic/v1/messages",
		},
		{
			name:    "Claude Code Mantle models → /api/llm-proxy/aws-bedrock/anthropic",
			baseURL: "https://bedrock-mantle.us-east-1.api.aws/anthropic/v1",
			reqPath: "v1/models",
			want:    "/anthropic/v1/models",
		},
		{
			name:    "OpenAI Bedrock Responses → /api/llm-proxy/aws-bedrock/openai/v1",
			baseURL: "https://bedrock-mantle.us-east-1.api.aws/openai/v1",
			reqPath: "v1/responses",
			want:    "/openai/v1/responses",
		},
		{
			name:    "OpenAI Bedrock models → /api/llm-proxy/aws-bedrock/openai/v1",
			baseURL: "https://bedrock-mantle.us-east-1.api.aws/openai/v1",
			reqPath: "v1/models",
			want:    "/openai/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := mustParseURL(tt.baseURL)

			req := httptest.NewRequest(http.MethodPost, "http://gateway.local/", nil)
			req.SetPathValue("path", tt.reqPath)

			proxyReq := &httputil.ProxyRequest{In: req, Out: req.Clone(req.Context())}
			llmRewriteRequest(*u)(proxyReq)
			req = proxyReq.Out

			if got := req.URL.Path; got != tt.want {
				t.Fatalf("URL.Path = %q, want %q", got, tt.want)
			}
		})
	}
}

// runModelListFilter builds a GET /v1/models response carrying body and runs it
// through filterModelListResponse exactly as the passthrough proxy would,
// returning the (possibly rewritten) response and its body.
func runModelListFilter(t *testing.T, statusCode int, allowAll bool, allowed map[string]bool, body string) (*http.Response, string) {
	t.Helper()

	resp := &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type":     []string{"application/json"},
			"Content-Encoding": []string{"gzip"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: &http.Request{URL: &url.URL{Path: "/v1/models"}},
	}

	if err := filterModelListResponse(resp, allowed, allowAll); err != nil {
		t.Fatalf("filterModelListResponse() error = %v", err)
	}

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return resp, string(out)
}

func modelDataIDs(body string) []string {
	var ids []string
	for _, r := range gjson.Get(body, "data.#.id").Array() {
		ids = append(ids, r.String())
	}
	return ids
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertCursor checks a pagination cursor field: want == nil means the field must
// be absent (deleted, not present-but-null); otherwise it must equal *want.
func assertCursor(t *testing.T, body, key string, want *string) {
	t.Helper()
	got := gjson.Get(body, key)
	if want == nil {
		if got.Exists() {
			t.Errorf("%s = %s, want field absent", key, got.Raw)
		}
		return
	}
	if got.String() != *want {
		t.Errorf("%s = %q, want %q", key, got.String(), *want)
	}
}

// assertEntriesPreserved checks that every surviving entry is byte-identical to
// the one in the upstream body — the byte-perfect fidelity guarantee.
func assertEntriesPreserved(t *testing.T, in, out string, ids []string) {
	t.Helper()
	for _, id := range ids {
		q := fmt.Sprintf("data.#(id==%q)", id)
		if want, got := gjson.Get(in, q).Raw, gjson.Get(out, q).Raw; want != got {
			t.Errorf("entry %q not preserved byte-for-byte:\n upstream = %s\n filtered = %s", id, want, got)
		}
	}
}

func assertFilteredHeaders(t *testing.T, resp *http.Response, out string) {
	t.Helper()
	if got := resp.Header.Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want removed", got)
	}
	if got, want := resp.Header.Get("Content-Length"), strconv.Itoa(len(out)); got != want {
		t.Errorf("Content-Length header = %q, want %q", got, want)
	}
	if resp.ContentLength != int64(len(out)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(out))
	}
}

func TestModifyResponse_FiltersAnthropicModelList(t *testing.T) {
	tests := []struct {
		name        string
		allowed     map[string]bool
		body        string
		wantIDs     []string
		wantFirstID *string
		wantLastID  *string
	}{
		{
			name:        "keeps allowed subset and repoints cursors to boundaries",
			allowed:     map[string]bool{"claude-opus-4": true, "claude-haiku-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"type":"model","id":"claude-sonnet-4"},{"type":"model","id":"claude-haiku-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-haiku-4"}`,
			wantIDs:     []string{"claude-opus-4", "claude-haiku-4"},
			wantFirstID: new("claude-opus-4"),
			wantLastID:  new("claude-haiku-4"),
		},
		{
			name:        "single middle survivor becomes both cursors",
			allowed:     map[string]bool{"claude-sonnet-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"type":"model","id":"claude-sonnet-4"},{"type":"model","id":"claude-haiku-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-haiku-4"}`,
			wantIDs:     []string{"claude-sonnet-4"},
			wantFirstID: new("claude-sonnet-4"),
			wantLastID:  new("claude-sonnet-4"),
		},
		{
			name:        "empty page on last page drops both cursors",
			allowed:     map[string]bool{},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-opus-4"}`,
			wantIDs:     nil,
			wantFirstID: nil,
			wantLastID:  nil,
		},
		{
			name:        "empty page with more pages retains upstream last_id",
			allowed:     map[string]bool{},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"}],"first_id":"claude-opus-4","has_more":true,"last_id":"page-cursor"}`,
			wantIDs:     nil,
			wantFirstID: nil,
			wantLastID:  new("page-cursor"),
		},
		{
			name:        "preserves entry bytes (key order and large integers)",
			allowed:     map[string]bool{"claude-sonnet-4": true},
			body:        `{"data":[{"type":"model","id":"claude-opus-4"},{"zzz_meta":1,"id":"claude-sonnet-4","created":1234567890123456789,"display_name":"Sonnet"}],"first_id":"claude-opus-4","has_more":false,"last_id":"claude-sonnet-4"}`,
			wantIDs:     []string{"claude-sonnet-4"},
			wantFirstID: new("claude-sonnet-4"),
			wantLastID:  new("claude-sonnet-4"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, http.StatusOK, false, tt.allowed, tt.body)

			if got := modelDataIDs(out); !sameStrings(got, tt.wantIDs) {
				t.Errorf("data ids = %v, want %v", got, tt.wantIDs)
			}
			assertCursor(t, out, "first_id", tt.wantFirstID)
			assertCursor(t, out, "last_id", tt.wantLastID)
			assertEntriesPreserved(t, tt.body, out, tt.wantIDs)
			assertFilteredHeaders(t, resp, out)
		})
	}
}

func TestModifyResponse_FiltersOpenAIModelList(t *testing.T) {
	tests := []struct {
		name    string
		allowed map[string]bool
		body    string
		wantIDs []string
	}{
		{
			name:    "keeps allowed subset and leaves the envelope alone",
			allowed: map[string]bool{"gpt-4o": true},
			body:    `{"object":"list","data":[{"id":"gpt-4o","object":"model","created":1686935002,"owned_by":"openai"},{"id":"gpt-3.5-turbo","object":"model","created":1677610602,"owned_by":"openai"}]}`,
			wantIDs: []string{"gpt-4o"},
		},
		{
			name:    "allowed nothing yields empty list",
			allowed: map[string]bool{},
			body:    `{"object":"list","data":[{"id":"gpt-4o","object":"model","created":1686935002}]}`,
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, http.StatusOK, false, tt.allowed, tt.body)

			if got := modelDataIDs(out); !sameStrings(got, tt.wantIDs) {
				t.Errorf("data ids = %v, want %v", got, tt.wantIDs)
			}
			if got := gjson.Get(out, "object").String(); got != "list" {
				t.Errorf("object = %q, want \"list\"", got)
			}
			// OpenAI lists carry no cursors; we must not invent them.
			assertCursor(t, out, "first_id", nil)
			assertCursor(t, out, "last_id", nil)
			assertEntriesPreserved(t, tt.body, out, tt.wantIDs)
			assertFilteredHeaders(t, resp, out)
		})
	}
}

func TestModifyResponse_ModelListPassthrough(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		allowAll   bool
		allowed    map[string]bool
		body       string
	}{
		{
			name:       "allow-all forwards the full list untouched",
			statusCode: http.StatusOK,
			allowAll:   true,
			body:       `{"data":[{"id":"a"},{"id":"b"}],"first_id":"a","has_more":false,"last_id":"b"}`,
		},
		{
			name:       "non-200 is forwarded untouched",
			statusCode: http.StatusInternalServerError,
			allowed:    map[string]bool{},
			body:       `{"type":"error","error":{"message":"boom"}}`,
		},
		{
			name:       "non-JSON body is forwarded untouched",
			statusCode: http.StatusOK,
			allowed:    map[string]bool{},
			body:       "not json at all",
		},
		{
			name:       "JSON without a data array is forwarded untouched",
			statusCode: http.StatusOK,
			allowed:    map[string]bool{},
			body:       `{"error":{"message":"nope"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, out := runModelListFilter(t, tt.statusCode, tt.allowAll, tt.allowed, tt.body)

			if out != tt.body {
				t.Errorf("body modified:\n got = %s\nwant = %s", out, tt.body)
			}
			// Passthrough must not strip transfer headers.
			if got := resp.Header.Get("Content-Encoding"); got != "gzip" {
				t.Errorf("Content-Encoding = %q, want gzip (untouched)", got)
			}
		})
	}
}

func TestRewriteAnthropicListCursors(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantFirstID   *string
		wantLastID    *string
		wantUnchanged bool
	}{
		{
			name:        "non-empty repoints to boundary ids",
			body:        `{"data":[{"id":"a"},{"id":"b"},{"id":"c"}],"first_id":"x","has_more":false,"last_id":"y"}`,
			wantFirstID: new("a"),
			wantLastID:  new("c"),
		},
		{
			name:          "openai shape without cursors is untouched",
			body:          `{"object":"list","data":[{"id":"a"}]}`,
			wantFirstID:   nil,
			wantLastID:    nil,
			wantUnchanged: true,
		},
		{
			name:        "empty page on last page drops both cursors",
			body:        `{"data":[],"first_id":"x","has_more":false,"last_id":"y"}`,
			wantFirstID: nil,
			wantLastID:  nil,
		},
		{
			name:        "empty page with more pages retains last_id",
			body:        `{"data":[],"first_id":"x","has_more":true,"last_id":"y"}`,
			wantFirstID: nil,
			wantLastID:  new("y"),
		},
		{
			name:        "only first_id present is the only field set",
			body:        `{"data":[{"id":"a"}],"first_id":"x"}`,
			wantFirstID: new("a"),
			wantLastID:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := rewriteAnthropicListCursors([]byte(tt.body))
			if err != nil {
				t.Fatalf("rewriteAnthropicListCursors() error = %v", err)
			}
			if tt.wantUnchanged && string(out) != tt.body {
				t.Errorf("body changed:\n got = %s\nwant = %s", out, tt.body)
			}
			assertCursor(t, string(out), "first_id", tt.wantFirstID)
			assertCursor(t, string(out), "last_id", tt.wantLastID)
		})
	}
}

func TestCopyBody(t *testing.T) {
	t.Run("restores body and returns a copy safe to modify", func(t *testing.T) {
		body := io.NopCloser(strings.NewReader("hello"))

		got, err := copyBody(&body)
		if err != nil {
			t.Fatalf("copyBody() error = %v", err)
		}
		if string(got) != "hello" {
			t.Fatalf("copyBody() = %q, want %q", got, "hello")
		}

		// Mutating the returned slice must not affect the restored body.
		got[0] = 'J'

		restored, err := io.ReadAll(body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if string(restored) != "hello" {
			t.Errorf("restored body = %q, want %q (unaffected by caller mutation)", restored, "hello")
		}
	})

	t.Run("works on an http.Response body field", func(t *testing.T) {
		resp := &http.Response{Body: io.NopCloser(strings.NewReader("payload"))}

		got, err := copyBody(&resp.Body)
		if err != nil {
			t.Fatalf("copyBody() error = %v", err)
		}
		if string(got) != "payload" {
			t.Fatalf("copyBody() = %q, want %q", got, "payload")
		}

		restored, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if string(restored) != "payload" {
			t.Errorf("restored body = %q, want %q", restored, "payload")
		}
	})
}

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
