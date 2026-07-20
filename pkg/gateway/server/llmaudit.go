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
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaycontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	defaultLLMAuditLogResponseCaptureLimit = 5 << 20 // 5MiB
	claudeCodeSessionIDHeader              = "X-Claude-Code-Session-Id"
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
			UserAgent:      req.UserAgent(),
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

func (r *llmAuditRecorder) setRequestBody(body []byte) {
	if r == nil {
		return
	}
	r.log.RequestBody = body
}

func (r *llmAuditRecorder) setPolicyModifiedRequestBody(body []byte) {
	if r == nil {
		return
	}
	r.log.PolicyModifiedRequestBody = body
	r.log.MessagePolicyTriggered = len(body) > 0
}

func (r *llmAuditRecorder) setClientSessionID(dialect nanobottypes.Dialect, headers http.Header, body []byte) {
	if r == nil {
		return
	}
	r.log.ClientSessionID = extractLLMClientSessionID(dialect, headers, body)
}

func (r *llmAuditRecorder) setReasoningEffort(modelProvider string, body []byte) {
	if r == nil {
		return
	}
	r.log.ReasoningEffort = extractLLMReasoningEffort(modelProvider, body)
}

func (r *llmAuditRecorder) recordResponse(resp *http.Response) {
	if r == nil || resp == nil {
		return
	}
	r.log.ResponseStatus = resp.StatusCode
	r.log.ResponseHeaders = redactedHeaders(resp.Header)
}

func (r *llmAuditRecorder) recordResponseStatus(status int) {
	if r == nil {
		return
	}
	r.log.ResponseStatus = status
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
		r.setOutcomeAndResponseStatus(err)
		c.LogLLMAuditEntry(r.log, r.responseStream.Bytes())
	})
}

func (r *llmAuditRecorder) setOutcomeAndResponseStatus(err error) {
	if r == nil {
		return
	}
	r.log.Outcome = types.LLMAuditOutcomeSuccess
	r.log.Error = ""

	if r.log.ResponseStatus >= http.StatusBadRequest {
		r.log.Outcome = types.LLMAuditOutcomeError
	}

	if err == nil {
		return
	}
	r.log.Error = err.Error()
	r.log.Outcome = types.LLMAuditOutcomeError
	if errors.Is(err, context.Canceled) {
		r.log.Outcome = types.LLMAuditOutcomeCanceled
	}

	if r.log.ResponseStatus == 0 {
		if errHTTP := (*apitypes.ErrHTTP)(nil); errors.As(err, &errHTTP) {
			r.log.ResponseStatus = errHTTP.Code
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			r.log.ResponseStatus = int(errStatus.ErrStatus.Code)
		} else {
			r.log.ResponseStatus = http.StatusInternalServerError
		}

		if r.log.ResponseStatus >= http.StatusBadRequest && r.log.Outcome != types.LLMAuditOutcomeCanceled {
			r.log.Outcome = types.LLMAuditOutcomeError
		}
	}
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

func redactedHeaders(headers http.Header) json.RawMessage {
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
	switch k {
	case "x-ratelimit-limit-tokens",
		"x-ratelimit-remaining-tokens",
		"x-ratelimit-reset-tokens",
		"anthropic-ratelimit-input-tokens-limit",
		"anthropic-ratelimit-input-tokens-remaining",
		"anthropic-ratelimit-input-tokens-reset",
		"anthropic-ratelimit-output-tokens-limit",
		"anthropic-ratelimit-output-tokens-remaining",
		"anthropic-ratelimit-output-tokens-reset",
		"anthropic-ratelimit-requests-limit",
		"anthropic-ratelimit-requests-remaining",
		"anthropic-ratelimit-requests-reset",
		"anthropic-ratelimit-tokens-limit",
		"anthropic-ratelimit-tokens-remaining",
		"anthropic-ratelimit-tokens-reset":
		return false
	}
	if k == "authorization" || k == "cookie" || k == "set-cookie" || k == "x-api-key" {
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "key") || strings.Contains(k, "credential")
}

func extractLLMClientSessionID(dialect nanobottypes.Dialect, headers http.Header, body []byte) string {
	if sessionID := headers.Get(claudeCodeSessionIDHeader); sessionID != "" {
		return sessionID
	}

	switch dialect {
	case nanobottypes.DialectOpenAIResponses:
		return gjson.GetBytes(body, "client_metadata.session_id").String()
	case nanobottypes.DialectAnthropicMessages:
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
