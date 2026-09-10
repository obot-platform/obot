package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/tidwall/gjson"
	"k8s.io/apiserver/pkg/authentication/user"
)

type TesterAudit struct {
	recorder *llmAuditRecorder
	client   *client.Client
}

type testerAuditReader struct {
	io.Reader
	recorder *llmAuditRecorder
	pending  []byte
	overflow bool
}

// NewTesterAudit reuses gateway audit capture and persistence without invoking
// gateway dispatch, token metering, or message policies. Caller identity comes
// from the inbound request and is never attached to the external request.
func NewTesterAudit(c *client.Client, inbound *http.Request, user user.Info, body []byte) *TesterAudit {
	if !c.LLMAuditLogEnabled() {
		return nil
	}

	recorder := newLLMAuditRecorder(inbound, user, defaultLLMAuditLogResponseCaptureLimit)
	recorder.setModel("model-proxy", "", mcptester.FallbackModel)
	recorder.log.ReasoningEffort = "high"
	recorder.setRequestBody(body)

	return &TesterAudit{recorder: recorder, client: c}
}

func (a *TesterAudit) Response(response *http.Response) io.Reader {
	if a == nil {
		return response.Body
	}

	a.recorder.recordResponse(response)

	return &testerAuditReader{Reader: response.Body, recorder: a.recorder}
}

func (r *testerAuditReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.recorder.captureResponseChunk(p[:n])

	for _, b := range p[:n] {
		if b == '\n' {
			if !r.overflow {
				r.recordUsage(r.pending)
			}

			r.pending = r.pending[:0]
			r.overflow = false
		} else if len(r.pending) < 1<<20 {
			r.pending = append(r.pending, b)
		} else {
			r.overflow = true
		}
	}

	if err == io.EOF && !r.overflow {
		r.recordUsage(r.pending)
		r.pending = nil
	}

	return n, err
}

func (a *TesterAudit) Finish(ctx context.Context, err error) {
	if a == nil {
		return
	}

	if ctx.Err() != nil {
		err = ctx.Err()
	}

	// Transport/provider errors can carry URLs or response text. Persist only
	// safe outcomes; the bounded response capture holds the model traffic.
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		err = errors.New("model proxy request failed")
	}

	a.recorder.finish(a.client, err)
}

func (r *testerAuditReader) recordUsage(line []byte) {
	data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
	if !ok {
		return
	}

	kind := gjson.GetBytes(data, "type").String()
	if kind != "response.completed" && kind != "response.incomplete" && kind != "response.failed" {
		return
	}

	usage := gjson.GetBytes(data, "response.usage")
	if usage.IsObject() {
		r.recorder.setTokenUsage(types.TokenUsage{
			InputTokens:  int(usage.Get("input_tokens").Int()),
			OutputTokens: int(usage.Get("output_tokens").Int()),
		})
	}
}
