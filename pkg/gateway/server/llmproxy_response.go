package server

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/tidwall/gjson"
)

func (r *llmResponseWriter) Header() http.Header { return r.header }

func (r *llmResponseWriter) WriteHeader(status int) {
	if r.status != 0 {
		return
	}
	r.status = status
	gatewayRoutingFrom(r.request.Context()).recordHeaders(r.header)
	r.stream = strings.Contains(r.header.Get("Content-Type"), "text/event-stream")
	if r.stream {
		r.commit()
	}
}

func (r *llmResponseWriter) commit() {
	if r.committed {
		return
	}
	r.committed = true
	if r.stream {
		r.header.Del("Content-Length")
		r.header.Del("Content-Encoding")
	}
	provider, model := r.header.Get("X-Bifrost-Provider"), r.header.Get("X-Bifrost-Resolved-Model")
	for name := range r.header {
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "x-bifrost-") || strings.HasPrefix(lower, "x-bf-") ||
			strings.HasPrefix(lower, "access-control-") || lower == "server" || lower == "vary" {
			r.header.Del(name)
		}
	}
	if provider != "" {
		r.header.Set(headerServedProvider, provider)
	}
	if model != "" {
		r.header.Set(headerServedModel, model)
	}
	resp := &http.Response{StatusCode: r.status, Header: r.header, Request: r.request}
	r.applyServedModel(resp)
	r.audit.recordResponse(resp)
	r.usage.recordResponseStatus(r.status)
	// Copy engine headers into the existing API headers, preserving CORS and rate limits.
	maps.Copy(r.w.Header(), r.header)
	r.w.WriteHeader(r.status)
}

func (r *llmResponseWriter) failed(err error) error {
	if err != nil && r.err == nil {
		r.err = err
		r.cancel()
	}
	return r.err
}

func (r *llmResponseWriter) Write(p []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if !r.stream {
		return r.body.Write(p)
	}
	// Only an unfinished line is retained. Flush cannot expose half of an internal
	// JSON field; complete lines retain comments, event names and original newlines.
	for rest := p; len(rest) > 0; {
		line, tail, complete := bytes.Cut(rest, []byte{'\n'})
		if len(r.pending)+len(line) > 100<<20 {
			return 0, r.failed(errors.New("LLM stream line exceeds 100 MB"))
		}
		r.pending = append(r.pending, line...)
		if !complete {
			break
		}
		r.pending = append(r.pending, '\n')
		if err := r.writeLine(r.pending); err != nil {
			return 0, err
		}
		r.pending = r.pending[:0]
		rest = tail
	}
	return len(p), nil
}

func (r *llmResponseWriter) writeLine(line []byte) error {
	if data, ok := bytes.CutPrefix(line, []byte("data: ")); ok && len(data) > 0 && data[0] == '{' {
		trimmed := bytes.TrimRight(data, "\r\n")
		gatewayRoutingFrom(r.request.Context()).recordBody(trimmed)
		event := stripEngineFields(trimmed)
		if gjson.GetBytes(event, "error").Exists() || gjson.GetBytes(event, "type").String() == "error" {
			event = llmengine.ScrubEngineNameBytes(event)
			r.streamError = bytes.Clone(event)
		}
		line = append(append([]byte("data: "), event...), data[len(trimmed):]...)
		if r.status == http.StatusOK {
			r.tokenUsageTracker.addTokenUsage(event)
		}
	}
	return r.writeClient(line)
}

func (r *llmResponseWriter) writeClient(data []byte) error {
	n, err := r.w.Write(data)
	if n > 0 {
		r.audit.captureResponseChunk(data[:n])
	}
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return r.failed(err)
}

// FlushError lets ResponseController propagate flush failures back into core's
// cancel-and-drain path. Unary headers remain pending until their rewrite finishes.
func (r *llmResponseWriter) FlushError() error {
	if r.err != nil {
		return r.err
	}
	if !r.stream {
		return nil
	}
	if err := http.NewResponseController(r.w).Flush(); err != nil {
		return r.failed(err)
	}
	if len(r.streamError) > 0 {
		r.audit.captureStreamError(r.streamError)
	}
	return nil
}

func (r *llmResponseWriter) finish() {
	if r.finished {
		return
	}
	if err := r.request.Context().Err(); err != nil {
		_ = r.failed(err)
	}
	if r.err != nil {
		r.finalize()
		return
	}
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	if !r.stream {
		body := r.body.Bytes()
		gatewayRoutingFrom(r.request.Context()).recordBody(body)
		body = stripEngineFields(body)
		if r.status >= http.StatusBadRequest {
			body = llmengine.ScrubEngineNameBytes(body)
		}
		resp := &http.Response{StatusCode: r.status, Header: r.header, Request: r.request, Body: io.NopCloser(bytes.NewReader(body))}
		if err := r.prepareUnary(resp); err != nil {
			// Reuse the public error formatter while headers and body are still local.
			r.header = make(http.Header)
			r.status = 0
			r.body.Reset()
			llmtypes.WriteError(r, r.request, "failed to prepare gateway response", http.StatusBadGateway)
			body = r.body.Bytes()
			_ = r.failed(err)
		} else {
			// Preparation helpers return byte-backed bodies; there is no upstream I/O here.
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
		r.header.Del("Content-Encoding")
		r.header.Set("Content-Length", strconv.Itoa(len(body)))
		r.commit()
		if r.status == http.StatusOK && !isModelsListRequest(r.request) {
			r.tokenUsageTracker.addTokenUsage(body)
		}
		_ = r.writeClient(body)
	} else if len(r.pending) > 0 && r.err == nil {
		if r.writeLine(r.pending) == nil {
			_ = r.FlushError()
		}
	}
	r.finalize()
}

func (r *llmResponseWriter) finalize() {
	if r.finished {
		return
	}
	r.finished = true
	// Price once after core has finished all plugin hooks, including blocked output.
	// Finish audit before releasing attempts so final route/findings remain available.
	if r.tokenUsageTracker != nil && r.status == http.StatusOK && meteredLLMEndpoint(strings.TrimPrefix(r.request.URL.Path, "/anthropic")) {
		usage := r.tokenUsageTracker.getTokenUsage()
		if r.gateway != nil {
			if cost, ok := r.gateway.cost(); ok {
				usage = r.withGatewayUsage(usage, cost)
			}
		}
		r.recordUsage(usage)
	} else if r.gateway != nil {
		if cost, ok := r.gateway.cost(); ok {
			r.recordUsage(r.withGatewayUsage(gatewaytypes.TokenUsage{}, cost))
		}
	}
	if r.err == nil {
		r.err = r.request.Context().Err()
		if r.err == nil && len(r.streamError) > 0 {
			r.err = errors.New("LLM gateway stream failed")
		}
	}
	r.audit.finish(r.client, r.err)
	r.usage.finish(r.err)
}
