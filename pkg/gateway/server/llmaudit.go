package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
	"uuid"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaycontext "github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/principal"
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
	once sync.Once
	log  types.LLMAuditLog
	// guardrailFindings, when set, returns what the LLM gateway's guardrails did to the
	// request. It is read when the entry is written, once the response has been delivered.
	guardrailFindings func() []apitypes.LLMAuditGuardrailFinding
	// route, when set, returns how the LLM gateway served the request. It is read when the
	// entry is written.
	route                 func() llmRoute
	responseStream        bytes.Buffer
	responseCaptureLimit  int
	responseCaptureFilled bool
}

func newLLMAuditRecorder(req *http.Request, user user.Info, responseCaptureLimit int) *llmAuditRecorder {
	now := time.Now()
	requestID := gatewaycontext.GetRequestID(req.Context())
	if requestID == "" {
		requestID = uuid.New().String()
	}

	userID := ""
	if user != nil {
		userID = user.GetUID()
	}
	var apiKeyID *uint
	apiKeyName := ""
	if attribution, ok := principal.APIKeyAttributionFromUser(user); ok {
		apiKeyID = &attribution.ID
		apiKeyName = attribution.Name
	}
	return &llmAuditRecorder{
		responseCaptureLimit: responseCaptureLimit,
		log: types.LLMAuditLog{
			ID:             uuid.New().String(),
			CreatedAt:      now,
			UserID:         userID,
			APIKeyID:       apiKeyID,
			APIKeyName:     apiKeyName,
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

func (r *llmAuditRecorder) setClientSessionID(dialect llmtypes.Dialect, headers http.Header, body []byte) {
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

func (r *llmAuditRecorder) setGuardrailFindings(findings func() []apitypes.LLMAuditGuardrailFinding) {
	if r == nil {
		return
	}
	r.guardrailFindings = findings
}

func (r *llmAuditRecorder) setRoute(route func() llmRoute) {
	if r == nil {
		return
	}
	r.route = route
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
	r.log.CacheReadTokens = usage.CacheReadTokens
	r.log.CacheWriteTokens = usage.CacheWriteTokens
	r.log.ReasoningTokens = usage.ThinkingTokens
	r.log.Spend = usage.TotalSpend
}

func (r *llmAuditRecorder) finish(c *client.Client, err error) {
	if r == nil || c == nil {
		return
	}
	r.once.Do(func() {
		r.log.Duration = time.Since(r.log.CreatedAt).Milliseconds()
		r.setOutcomeAndResponseStatus(err)
		if r.guardrailFindings != nil {
			r.log.GuardrailFindings = r.guardrailFindings()
		}
		if r.route != nil {
			route := r.route()
			r.log.ServedProvider = route.Provider
			r.log.ServedModel = route.Model
			r.log.ProviderKey = route.Key
			r.log.Fallback = route.Fallback
			r.log.ClaudeSubscription = route.Subscription
		}
		c.LogLLMAuditEntry(r.log, r.responseStream.Bytes())
	})
}

func (r *llmAuditRecorder) setOutcomeAndResponseStatus(err error) {
	if r == nil {
		return
	}
	r.log.Outcome, r.log.Error, r.log.ResponseStatus = llmOutcome(r.log.ResponseStatus, err)
}

// llmOutcome is the outcome of an LLM request (success, error or canceled), given the status
// of its response, if it got one, and why it failed, if it did. It also returns the error
// message and the response status, which comes from the error when there was no response.
func llmOutcome(status int, err error) (outcome, message string, responseStatus int) {
	outcome, responseStatus = types.LLMAuditOutcomeSuccess, status
	if status >= http.StatusBadRequest {
		outcome = types.LLMAuditOutcomeError
	}
	if err == nil {
		return outcome, "", responseStatus
	}
	message, outcome = err.Error(), types.LLMAuditOutcomeError
	if errors.Is(err, context.Canceled) {
		outcome = types.LLMAuditOutcomeCanceled
	}
	if responseStatus == 0 {
		if errHTTP := (*apitypes.ErrHTTP)(nil); errors.As(err, &errHTTP) {
			responseStatus = errHTTP.Code
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			responseStatus = int(errStatus.ErrStatus.Code)
		} else {
			responseStatus = http.StatusInternalServerError
		}
	}
	return outcome, message, responseStatus
}

func redactedHeaders(headers http.Header) json.RawMessage {
	out := make(http.Header, len(headers))
	for k, values := range headers {
		if shouldRedactHeader(k) {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		out[k] = slices.Clone(values)
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
	if k == "authorization" || k == "cookie" || k == "set-cookie" || k == "x-api-key" || k == "x-obot-machine-fingerprint" {
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "key") || strings.Contains(k, "credential")
}

func extractLLMClientSessionID(dialect llmtypes.Dialect, headers http.Header, body []byte) string {
	if sessionID := headers.Get(claudeCodeSessionIDHeader); sessionID != "" {
		return sessionID
	}

	switch dialect {
	case llmtypes.DialectOpenAIResponses:
		return gjson.GetBytes(body, "client_metadata.session_id").String()
	case llmtypes.DialectAnthropicMessages:
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
	case system.LLMGatewayModelProvider:
		if effort := gjson.GetBytes(body, "reasoning.effort").String(); effort != "" {
			return effort
		}
		if effort := gjson.GetBytes(body, "reasoning_effort").String(); effort != "" {
			return effort
		}
		return gjson.GetBytes(body, "output_config.effort").String()
	default:
		return ""
	}
}
