package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/streaming"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

type auditMarshalProbe func()

func (p auditMarshalProbe) MarshalJSON() ([]byte, error) {
	p()
	return nil, errors.New("serialization probe")
}

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
	req.Header.Set("User-Agent", "PDi/JS 0.94.0")
	recorder := newLLMAuditRecorder(req, nil, 5<<20)

	if recorder.log.RequestPath != "/api/llm-provider/openai/v1/responses" {
		t.Fatalf("expected request path, got %q", recorder.log.RequestPath)
	}
	if recorder.log.RequestMethod != http.MethodPost {
		t.Fatalf("expected request method, got %q", recorder.log.RequestMethod)
	}
	if recorder.log.UserAgent != "PDi/JS 0.94.0" {
		t.Fatalf("expected raw user agent, got %q", recorder.log.UserAgent)
	}
	if recorder.log.APIKeyID != nil || recorder.log.APIKeyName != "" {
		t.Fatalf("non-API-key request gained attribution: ID=%v name=%q", recorder.log.APIKeyID, recorder.log.APIKeyName)
	}
}

func TestNewLLMAuditRecorderCapturesAPIKeyAttribution(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	requestUser := &user.DefaultInfo{
		UID: "7",
		Extra: map[string][]string{
			principal.APIKeyIDExtra:   {"42"},
			principal.APIKeyNameExtra: {"CLI token"},
		},
	}

	recorder := newLLMAuditRecorder(req, requestUser, 5<<20)

	if recorder.log.APIKeyID == nil || *recorder.log.APIKeyID != 42 {
		t.Fatalf("APIKeyID = %v, want 42", recorder.log.APIKeyID)
	}
	if recorder.log.APIKeyName != "CLI token" {
		t.Fatalf("APIKeyName = %q, want CLI token", recorder.log.APIKeyName)
	}
}

func TestNewLLMAuditRecorderCapturesMaskedUnnamedAPIKeyAttribution(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/llm-provider/openai/v1/responses", nil)
	requestUser := &user.DefaultInfo{
		UID: "7",
		Extra: map[string][]string{
			principal.APIKeyIDExtra:   {"42"},
			principal.APIKeyNameExtra: {"ok1-7-42-*****"},
		},
	}

	recorder := newLLMAuditRecorder(req, requestUser, 5<<20)

	if recorder.log.APIKeyID == nil || *recorder.log.APIKeyID != 42 {
		t.Fatalf("APIKeyID = %v, want 42", recorder.log.APIKeyID)
	}
	if recorder.log.APIKeyName != "ok1-7-42-*****" {
		t.Fatalf("APIKeyName = %q, want masked key identifier", recorder.log.APIKeyName)
	}
}

func TestLLMAuditRecorderSetOutcome(t *testing.T) {
	for _, tt := range []struct {
		name           string
		err            error
		responseStatus int
		outcome        string
		wantErr        string
		wantStatus     int
	}{
		{
			name:    "success even if request context is canceled after response",
			outcome: types.LLMAuditOutcomeSuccess,
		},
		{
			name:           "HTTP error response",
			responseStatus: http.StatusForbidden,
			outcome:        types.LLMAuditOutcomeError,
			wantStatus:     http.StatusForbidden,
		},
		{
			name:       "actual context cancellation error",
			err:        context.Canceled,
			outcome:    types.LLMAuditOutcomeCanceled,
			wantErr:    context.Canceled.Error(),
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "actual proxy error",
			err:        errors.New("proxy failed"),
			outcome:    types.LLMAuditOutcomeError,
			wantErr:    "proxy failed",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "application HTTP error",
			err:        apitypes.NewErrForbidden("model is not allowed"),
			outcome:    types.LLMAuditOutcomeError,
			wantErr:    "error code 403 (Forbidden): model is not allowed",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrapped application HTTP error",
			err:        errors.Join(errors.New("request rejected"), apitypes.NewErrBadRequest("invalid model")),
			outcome:    types.LLMAuditOutcomeError,
			wantErr:    "request rejected\nerror code 400 (Bad Request): invalid model",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Kubernetes status error",
			err:        apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "models"}, "missing"),
			outcome:    types.LLMAuditOutcomeError,
			wantErr:    `models.obot.obot.ai "missing" not found`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:           "recorded response status is preserved",
			err:            errors.New("response body failed"),
			responseStatus: http.StatusTooManyRequests,
			outcome:        types.LLMAuditOutcomeError,
			wantErr:        "response body failed",
			wantStatus:     http.StatusTooManyRequests,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &llmAuditRecorder{}
			recorder.recordResponseStatus(tt.responseStatus)
			recorder.setOutcomeAndResponseStatus(tt.err)
			if recorder.log.Outcome != tt.outcome {
				t.Fatalf("expected outcome %q, got %q", tt.outcome, recorder.log.Outcome)
			}
			if recorder.log.Error != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, recorder.log.Error)
			}
			if recorder.log.ResponseStatus != tt.wantStatus {
				t.Fatalf("expected response status %d, got %d", tt.wantStatus, recorder.log.ResponseStatus)
			}
		})
	}
}

func TestExtractLLMClientSessionID(t *testing.T) {
	for _, tt := range []struct {
		name    string
		dialect llmtypes.Dialect
		headers http.Header
		body    string
		want    string
	}{
		{
			name:    "Claude Code session header preferred over body",
			dialect: llmtypes.DialectAnthropicMessages,
			headers: http.Header{claudeCodeSessionIDHeader: []string{"header-session"}},
			body:    `{"metadata":{"user_id":"{\"session_id\":\"body-session\"}"}}`,
			want:    "header-session",
		},
		{
			name:    "OpenAI dialect client metadata session id",
			dialect: llmtypes.DialectOpenAIResponses,
			body:    `{"client_metadata":{"session_id":"openai-session"}}`,
			want:    "openai-session",
		},
		{
			name:    "OpenAI dialect ignores Codex metadata fallback",
			dialect: llmtypes.DialectOpenAIResponses,
			body:    `{"client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"ignored\"}"}}`,
		},
		{
			name:    "Anthropic dialect metadata user id session id",
			dialect: llmtypes.DialectAnthropicMessages,
			body:    `{"metadata":{"user_id":"{\"session_id\":\"claude-session\"}"}}`,
			want:    "claude-session",
		},
		{
			name:    "Anthropic dialect malformed metadata user id",
			dialect: llmtypes.DialectAnthropicMessages,
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
			modelProvider: system.LLMGatewayModelProvider,
			body:          `{"reasoning":{"effort":"medium"}}`,
			want:          "medium",
		},
		{
			name:          "anthropic output effort",
			modelProvider: system.LLMGatewayModelProvider,
			body:          `{"output_config":{"effort":"high"}}`,
			want:          "high",
		},
		{
			name:          "anthropic thinking type is not effort",
			modelProvider: system.LLMGatewayModelProvider,
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

func TestLLMAuditRecorderCapsResponseCapture(t *testing.T) {
	r := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/responses", nil), nil, 5)
	r.captureResponseChunk([]byte("hello"))
	r.captureResponseChunk([]byte(" world"))
	r.finishBody()
	if len(r.log.ResponseBody) != 0 || !r.responseCaptureFilled {
		t.Fatal("oversized body must be omitted")
	}
}

func TestLLMAuditRecordsSpendAndCachedTokens(t *testing.T) {
	recorder := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", nil), nil, 5<<20)
	r := &llmResponseWriter{audit: recorder}
	r.recordUsage(types.TokenUsage{
		InputTokens:      3132,
		CacheReadTokens:  3072,
		CacheWriteTokens: 10,
		OutputTokens:     40,
		ThinkingTokens:   12,
		TotalTokens:      3172,
		TotalSpend:       0.0002394,
	})

	entry := types.ConvertLLMAuditLog(recorder.log)
	if entry.InputTokens != 3132 || entry.CacheReadTokens != 3072 || entry.CacheWriteTokens != 10 ||
		entry.OutputTokens != 40 || entry.ReasoningTokens != 12 || entry.Spend != 0.0002394 {
		t.Fatalf("the audit entry doesn't carry the request's usage and spend: %+v", entry)
	}
}

func TestAuditBifrostChatCapture(t *testing.T) {
	a := streaming.NewAccumulator(nil, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer a.Cleanup()
	r := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/chat/completions", nil), nil, 5<<20)
	r.stream = true
	for i, text := range []string{"hello", " world"} {
		var resp schemas.BifrostChatResponse
		if err := json.Unmarshal([]byte(`{"id":"chat_1","object":"chat.completion.chunk","model":"test","choices":[{"index":0,"delta":{"role":"assistant","content":"`+text+`"}}]}`), &resp); err != nil {
			t.Fatal(err)
		}
		resp.ExtraFields.RequestType = schemas.ChatCompletionStreamRequest
		resp.ExtraFields.ChunkIndex = i
		r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostChatResponse: &resp}, a)
	}
	r.finishBody()
	if got := gjson.GetBytes(r.log.ResponseBody, "choices.0.message.content").String(); got != "hello world" {
		t.Fatalf("body = %s", r.log.ResponseBody)
	}
	if gjson.GetBytes(r.log.ResponseBody, "extra_fields").Exists() {
		t.Fatal("internal fields persisted")
	}
	if r.log.ResponseID != "chat_1" {
		t.Fatalf("ID = %s", r.log.ResponseID)
	}
	if a.GetAccumulatedResponse(r.log.ID) != nil {
		t.Fatal("capture was not cleaned up")
	}
	r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostChatResponse: &schemas.BifrostChatResponse{}}, a)
	if a.GetAccumulatedResponse(r.log.ID) != nil {
		t.Fatal("late callback recreated capture")
	}
}

func TestAuditBifrostResponsesDialects(t *testing.T) {
	for _, path := range []string{"/responses", "/messages"} {
		t.Run(path, func(t *testing.T) {
			a := streaming.NewAccumulator(nil, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
			defer a.Cleanup()
			r := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, path, nil), nil, 5<<20)
			r.stream = true
			var resp schemas.BifrostResponsesStreamResponse
			raw := `{"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","model":"test","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"approved"}]}]}}`
			if err := json.Unmarshal([]byte(raw), &resp); err != nil {
				t.Fatal(err)
			}
			resp.ExtraFields.RequestType = schemas.ResponsesStreamRequest
			r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostResponsesStreamResponse: &resp}, a)
			r.finishBody()
			if !strings.Contains(string(r.log.ResponseBody), "approved") {
				t.Fatalf("body = %s", r.log.ResponseBody)
			}
			if path == "/messages" && gjson.GetBytes(r.log.ResponseBody, "type").String() != "message" {
				t.Fatalf("wrong dialect: %s", r.log.ResponseBody)
			}
		})
	}
}

func TestAuditCaptureSkipsSerializationWhenClosedOrFull(t *testing.T) {
	for _, tc := range []struct {
		name        string
		closed      bool
		full        bool
		wantMarshal bool
	}{
		{
			name:        "active",
			wantMarshal: true,
		},
		{
			name:   "closed",
			closed: true,
		},
		{
			name: "full",
			full: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &llmAuditRecorder{
				closed:                tc.closed,
				responseCaptureFilled: tc.full,
			}
			marshaled := false
			chunk := &schemas.BifrostStreamChunk{BifrostChatResponse: &schemas.BifrostChatResponse{
				ExtraFields: schemas.BifrostResponseExtraFields{
					RawResponse: auditMarshalProbe(func() { marshaled = true }),
				},
			}}
			r.observeResponseChunk(chunk, nil)
			if marshaled != tc.wantMarshal {
				t.Fatalf("serialized chunk = %v, want %v", marshaled, tc.wantMarshal)
			}
		})
	}
}

func TestAuditCaptureConcurrentCloseAndOverflow(t *testing.T) {
	a := streaming.NewAccumulator(nil, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer a.Cleanup()
	r := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/responses", nil), nil, 5)
	var resp schemas.BifrostChatResponse
	if err := json.Unmarshal([]byte(`{"id":"chat_1","choices":[{"index":0,"delta":{"content":"too large"}}]}`), &resp); err != nil {
		t.Fatal(err)
	}
	resp.ExtraFields.RequestType = schemas.ChatCompletionStreamRequest
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostChatResponse: &resp}, a)
			r.captureResponseChunk([]byte("too large"))
		})
	}
	wg.Wait()
	r.finishBody()
	if !r.responseCaptureFilled || len(r.log.ResponseBody) != 0 || a.GetAccumulatedResponse(r.log.ID) != nil {
		t.Fatal("oversized capture retained data")
	}
}

func TestAuditCaptureConcurrentFinish(t *testing.T) {
	a := streaming.NewAccumulator(nil, bifrost.NewDefaultLogger(schemas.LogLevelWarn))
	defer a.Cleanup()
	r := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/chat/completions", nil), nil, 5<<20)
	r.stream = true
	var resp schemas.BifrostChatResponse
	if err := json.Unmarshal([]byte(`{"id":"chat_1","choices":[{"index":0,"delta":{"content":"hello"}}]}`), &resp); err != nil {
		t.Fatal(err)
	}
	resp.ExtraFields.RequestType = schemas.ChatCompletionStreamRequest
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			<-start
			r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostChatResponse: &resp}, a)
			r.captureResponseChunk([]byte("hello"))
			r.captureStreamError([]byte(`{"error":"stream ended"}`))
		})
	}
	wg.Go(func() {
		<-start
		r.finishBody()
	})
	close(start)
	wg.Wait()
	if !r.closed || a.GetAccumulatedResponse(r.log.ID) != nil {
		t.Fatal("capture did not close and release its accumulator")
	}
	if len(r.log.ResponseBody) > 0 && !json.Valid(r.log.ResponseBody) {
		t.Fatal("concurrent finalization produced invalid JSON")
	}
	r.observeResponseChunk(&schemas.BifrostStreamChunk{BifrostChatResponse: &resp}, a)
	if a.GetAccumulatedResponse(r.log.ID) != nil {
		t.Fatal("late callback recreated closed capture")
	}
}
