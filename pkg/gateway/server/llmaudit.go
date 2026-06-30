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
	"github.com/tidwall/gjson"
	"k8s.io/apiserver/pkg/authentication/user"
)

type llmAuditRecorder struct {
	once sync.Once
	ctx  context.Context
	log  types.LLMAuditLog
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

	return &llmAuditRecorder{
		ctx: req.Context(),
		log: types.LLMAuditLog{
			ID:             uuid.NewString(),
			CreatedAt:      now,
			UserID:         userID,
			RequestHeaders: redactedHeaders(req.Header),
			RequestID:      requestID,
			UserAgent:      req.UserAgent(),
			ClientIP:       requestinfo.GetSourceIP(req),
		},
	}
}

func (r *llmAuditRecorder) setRequestBody(body []byte) {
	if r != nil {
		r.log.RequestBody = string(body)
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
	r.log.ResponseBody += string(p)
	r.log.ResponseText += extractLLMResponseText(p)
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
		if r.log.ResponseText == "" {
			r.log.ResponseText = extractLLMResponseText([]byte(r.log.ResponseBody))
		}
		r.log.Outcome = "success"
		if errors.Is(r.ctx.Err(), context.Canceled) {
			r.log.Outcome = "canceled"
			r.log.Error = r.ctx.Err().Error()
		} else if err != nil {
			r.log.Outcome = "error"
			r.log.Error = err.Error()
		}

		insertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.InsertLLMAuditLog(insertCtx, &r.log); err != nil {
			log.Warnf("failed to insert LLM audit log: %v", err)
		}
	})
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

func extractLLMResponseText(p []byte) string {
	var out strings.Builder
	for line := range strings.SplitSeq(string(p), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "data: ")
		if line == "" || line == "[DONE]" || !gjson.Valid(line) {
			continue
		}
		if delta := gjson.Get(line, "delta"); gjson.Get(line, "type").String() == "response.output_text.delta" && delta.Exists() {
			out.WriteString(delta.String())
		}
		if text := gjson.Get(line, "delta.text"); gjson.Get(line, "type").String() == "content_block_delta" && text.Exists() {
			out.WriteString(text.String())
		}
		gjson.Get(line, "choices.#.delta.content").ForEach(func(_, value gjson.Result) bool {
			out.WriteString(value.String())
			return true
		})
		gjson.Get(line, "output.#.content.#.text").ForEach(func(_, value gjson.Result) bool {
			out.WriteString(value.String())
			return true
		})
		gjson.Get(line, "content.#.text").ForEach(func(_, value gjson.Result) bool {
			out.WriteString(value.String())
			return true
		})
	}
	return out.String()
}
