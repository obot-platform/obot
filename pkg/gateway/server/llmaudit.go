package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaycontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	defaultLLMAuditLogResponseCaptureLimit = 5 << 20 // 5MiB

	llmAuditClientClaudeCode = "claude-code"
	llmAuditClientCodex      = "codex"

	llmAuditUserAgentClaudeCode = "claude-code"
	llmAuditUserAgentClaudeCLI  = "claude-cli"
	llmAuditUserAgentCodexCLI   = "codex_cli_rs"
	llmAuditUserAgentCodexTUI   = "codex-tui"
)

type llmAuditRecorder struct {
	once                  sync.Once
	log                   types.LLMAuditLog
	responseStream        bytes.Buffer
	responseCaptureLimit  int
	responseCaptureFilled bool
}

func newLLMAuditRecorder(req *http.Request, user user.Info, responseCaptureLimit int) *llmAuditRecorder {
	now := time.Now()
	requestID := gatewaycontext.GetRequestID(req.Context())
	if requestID == "" {
		requestID = uuid.NewString()
	}

	userID := ""
	if user != nil {
		userID = user.GetUID()
	}
	clientName, clientVersion := parseLLMClientUserAgent(req.UserAgent())

	return &llmAuditRecorder{
		responseCaptureLimit: responseCaptureLimit,
		log: types.LLMAuditLog{
			ID:             uuid.NewString(),
			CreatedAt:      now,
			UserID:         userID,
			RequestPath:    req.URL.Path,
			RequestMethod:  req.Method,
			RequestHeaders: redactedHeaders(req.Header),
			RequestID:      requestID,
			Client:         clientName,
			ClientVersion:  clientVersion,
			ClientIP:       requestinfo.GetSourceIP(req),
		},
	}
}

func (r *llmAuditRecorder) setModel(modelProvider, modelID, targetModel string) {
	if r == nil {
		return
	}
	r.log.ModelProvider = modelProvider
	r.log.ModelID = modelID
	r.log.TargetModel = targetModel
}

func (r *llmAuditRecorder) recordResponse(resp *http.Response) {
	if r == nil || resp == nil {
		return
	}
	r.log.ResponseStatus = resp.StatusCode
	r.log.ResponseHeaders = redactedHeaders(resp.Header)
}

func (r *llmAuditRecorder) captureResponseChunk(p []byte) {
	if r == nil || len(p) == 0 {
		return
	}
	if r.responseCaptureFilled {
		return
	}
	remaining := r.responseCaptureLimit - r.responseStream.Len()
	if remaining <= 0 {
		r.responseCaptureFilled = true
		return
	}
	if len(p) > remaining {
		p = p[:remaining]
		r.responseCaptureFilled = true
	}
	_, _ = r.responseStream.Write(p)
}

func (r *llmAuditRecorder) setTokenUsage(usage types.TokenUsage) {
	if r == nil {
		return
	}
	r.log.InputTokens = usage.InputTokens
	r.log.OutputTokens = usage.OutputTokens
}

func (r *llmAuditRecorder) finish(c *client.Client, err error) {
	if r == nil || c == nil {
		return
	}
	r.once.Do(func() {
		r.log.Duration = time.Since(r.log.CreatedAt).Milliseconds()
		r.setOutcome(err)
		c.LogLLMAuditEntry(r.log, r.responseStream.String())
	})
}

func (r *llmAuditRecorder) setOutcome(err error) {
	if r == nil {
		return
	}
	r.log.Outcome = types.LLMAuditOutcomeSuccess
	r.log.Error = ""
	if err == nil {
		return
	}
	r.log.Error = err.Error()
	if errors.Is(err, context.Canceled) {
		r.log.Outcome = types.LLMAuditOutcomeCanceled
		return
	}
	r.log.Outcome = types.LLMAuditOutcomeError
}

type llmAuditResponseBody struct {
	body   io.ReadCloser
	audit  *llmAuditRecorder
	client *client.Client
}

func (r *llmAuditResponseBody) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.audit.captureResponseChunk(p[:n])
	}
	return n, err
}

func (r *llmAuditResponseBody) Close() error {
	err := r.body.Close()
	r.audit.finish(r.client, err)
	return err
}

func redactedHeaders(headers http.Header) string {
	out := make(http.Header, len(headers))
	for k, values := range headers {
		if shouldRedactHeader(k) {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		out[k] = append([]string(nil), values...)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func shouldRedactHeader(key string) bool {
	k := strings.ToLower(key)
	if k == "authorization" || k == "cookie" || k == "set-cookie" || k == "x-api-key" {
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "key") || strings.Contains(k, "credential")
}

func parseLLMClientUserAgent(userAgent string) (string, string) {
	token, _, _ := strings.Cut(strings.TrimSpace(userAgent), " ")
	if token == "" {
		return "", ""
	}
	name, version, ok := strings.Cut(token, "/")
	if !ok {
		name = token
		version = ""
	}

	switch name {
	case llmAuditUserAgentClaudeCode, llmAuditUserAgentClaudeCLI:
		name = llmAuditClientClaudeCode
	case llmAuditUserAgentCodexCLI, llmAuditUserAgentCodexTUI:
		name = llmAuditClientCodex
	}
	return name, version
}

func extractLLMClientSessionID(modelProvider string, body []byte) string {
	switch modelProvider {
	case system.OpenAIModelProvider:
		return gjson.GetBytes(body, "client_metadata.session_id").String()
	case system.AnthropicModelProvider:
		userID := gjson.GetBytes(body, "metadata.user_id").String()
		if !gjson.Valid(userID) {
			return ""
		}
		return gjson.Get(userID, "session_id").String()
	default:
		return ""
	}
}

func extractLLMReasoningEffort(modelProvider string, body []byte) string {
	switch modelProvider {
	case system.OpenAIModelProvider:
		return gjson.GetBytes(body, "reasoning.effort").String()
	case system.AnthropicModelProvider:
		return gjson.GetBytes(body, "output_config.effort").String()
	default:
		return ""
	}
}
