package server

import (
	"context"
	"encoding/json"
	"errors"
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
	"gorm.io/datatypes"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	llmAuditClientClaudeCode = "claude-code"
	llmAuditClientCodex      = "codex"

	llmAuditUserAgentClaudeCode = "claude-code"
	llmAuditUserAgentClaudeCLI  = "claude-cli"
	llmAuditUserAgentCodexCLI   = "codex_cli_rs"
	llmAuditUserAgentCodexTUI   = "codex-tui"
)

type llmAuditRecorder struct {
	once        sync.Once
	log         types.LLMAuditLog
	accumulator *llmResponseAccumulator
}

func newLLMAuditRecorder(req *http.Request, user user.Info) *llmAuditRecorder {
	now := time.Now().UTC()
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

func (r *llmAuditRecorder) setRequestBody(body []byte) {
	if r != nil {
		r.log.RequestBody = string(body)
	}
}

func (r *llmAuditRecorder) setClientSessionID(sessionID string) {
	if r != nil {
		r.log.ClientSessionID = sessionID
	}
}

func (r *llmAuditRecorder) setReasoningEffort(reasoningEffort string) {
	if r != nil {
		r.log.ReasoningEffort = reasoningEffort
	}
}

func (r *llmAuditRecorder) setModel(modelProvider, modelID, targetModel string) {
	if r == nil {
		return
	}
	r.log.ModelProvider = modelProvider
	r.log.ModelID = modelID
	r.log.TargetModel = targetModel
	if r.accumulator == nil {
		r.accumulator = newLLMResponseAccumulator(modelProvider)
	}
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
	if r.accumulator == nil {
		r.accumulator = newLLMResponseAccumulator(r.log.ModelProvider)
	}
	r.accumulator.Write(p)
	if r.log.ResponseID == "" {
		r.log.ResponseID = r.accumulator.ResponseID()
	}
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
		if r.accumulator == nil {
			r.accumulator = newLLMResponseAccumulator(r.log.ModelProvider)
		}
		r.log.ResponseBody = r.accumulator.JSON()
		if r.log.ResponseID == "" {
			r.log.ResponseID = r.accumulator.ResponseID()
		}
		r.setOutcome(err)

		insertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.InsertLLMAuditLog(insertCtx, &r.log); err != nil {
			log.Warnf("failed to insert LLM audit log: %v", err)
		}
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

type llmAuditReadCloser struct {
	ioCloser ioReadCloser
	audit    *llmAuditRecorder
	client   *client.Client
}

type ioReadCloser interface {
	Read([]byte) (int, error)
	Close() error
}

func (r *llmAuditReadCloser) Read(p []byte) (int, error) {
	n, err := r.ioCloser.Read(p)
	if n > 0 {
		r.audit.captureResponseChunk(p[:n])
	}
	return n, err
}

func (r *llmAuditReadCloser) Close() error {
	err := r.ioCloser.Close()
	r.audit.finish(r.client, err)
	return err
}

func redactedHeaders(headers http.Header) datatypes.JSON {
	out := make(http.Header, len(headers))
	for k, values := range headers {
		if shouldRedactHeader(k) {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		out[k] = append([]string(nil), values...)
	}
	b, _ := json.Marshal(out)
	return b
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
