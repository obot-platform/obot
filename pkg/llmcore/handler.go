package llmcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/streaming"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	dialectResponses dialect = iota
	dialectMessages
	dialectChat

	// headerServedProvider and headerServedModel name the provider and model that served a
	// response, which routing rules and fallbacks can make differ from the requested ones.
	headerServedProvider = "X-Obot-LLM-Provider"
	headerServedModel    = "X-Obot-LLM-Model"

	maxRequestBodyBytes = 100 << 20
)

// engineName matches a library error prefix, preserving provider text that merely mentions it.
var (
	engineName = regexp.MustCompile(`(?i)^bifrost\b`)
)

type dialect int
type responseObserverKey struct{}
type streamObserverKey struct{}

// ServeHTTP serves the OpenAI Responses API at /v1/responses and the Anthropic Messages API at
// /v1/messages (and /anthropic/v1/messages, the path Obot's proxy uses for the engine today).
// Either API can reach any provider's models: the engine translates.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d := dialectResponses
	if strings.Contains(r.URL.Path, "/messages") {
		d = dialectMessages
	}

	credential, valid := subscriptionCredential(r.Header)
	if !valid {
		writeError(w, d, http.StatusForbidden, "permission_denied", "Caller credentials require a single supported subscription bearer and matching CLI User-Agent; use the authenticated Obot proxy")
		return
	}

	if credential.token != "" {
		lifetime := &e.subscriptionLifetime
		// TryRLock refuses new readers once retirement is waiting. Hold through
		// stream drain and handler return, beyond core's pre-hook/enqueue window.
		if !lifetime.TryRLock() {
			writeError(w, d, http.StatusServiceUnavailable, "api_error", "Subscription provider is being reconfigured; retry the request")
			return
		}
		defer lifetime.RUnlock()
		r = r.WithContext(context.WithValue(r.Context(), subscriptionTokenKey{}, credential))
	}

	e.mux.ServeHTTP(w, r)
}

func (e *Engine) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		e.serveCount(w, r, dialectMessages)
	})
	mux.HandleFunc("POST /anthropic/v1/messages/count_tokens", func(w http.ResponseWriter, r *http.Request) {
		e.serveCount(w, r, dialectMessages)
	})
	mux.HandleFunc("POST /v1/responses/input_tokens", func(w http.ResponseWriter, r *http.Request) {
		e.serveCount(w, r, dialectResponses)
	})
	mux.HandleFunc("POST /v1/chat/completions", e.serveChat)
	mux.HandleFunc("POST /v1/responses", func(w http.ResponseWriter, r *http.Request) {
		e.serve(w, r, dialectResponses)
	})
	mux.HandleFunc("POST /v1/messages", func(w http.ResponseWriter, r *http.Request) {
		e.serve(w, r, dialectMessages)
	})
	mux.HandleFunc("POST /anthropic/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		e.serve(w, r, dialectMessages)
	})
	// Restrict the fallback to POST so other methods receive the mux's 405 response.
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, dialectResponses, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("unsupported endpoint %s", r.URL.Path))
	})
	return mux
}

func (e *Engine) serve(w http.ResponseWriter, r *http.Request, d dialect) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeBodyReadError(w, d, err)
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	req, stream, err := e.decodeResponsesRequest(ctx, body, d)
	if err != nil {
		writeError(w, d, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	if stream {
		e.serveStream(w, ctx, req, d)
		return
	}

	resp, bifrostErr := e.client.ResponsesRequest(ctx, req)
	if bifrostErr != nil {
		observeError(ctx, bifrostErr)
		writeEngineError(w, d, bifrostErr)
		return
	}
	observeResponse(ctx, resp.ExtraFields)
	setServed(w.Header(), resp.ExtraFields)
	switch d {
	case dialectResponses:
		writeJSON(w, d, http.StatusOK, resp.WithDefaults())
	case dialectMessages:
		writeJSON(w, d, http.StatusOK, anthropic.ToAnthropicResponsesResponse(ctx, resp))
	}
}

// Inference and token counting must use the same dialect conversion and
// replica-local model parsing, including caller fallbacks.
func (e *Engine) decodeResponsesRequest(ctx *schemas.BifrostContext, body []byte, d dialect) (*schemas.BifrostResponsesRequest, bool, error) {
	var (
		req    *schemas.BifrostResponsesRequest
		stream *bool
	)
	if d == dialectMessages {
		var in anthropic.AnthropicMessageRequest
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, false, fmt.Errorf("invalid request body: %w", err)
		}
		req, stream = in.ToBifrostResponsesRequest(ctx), in.Stream
		req.Provider, req.Model, req.Fallbacks = e.normalizeRequestModels(ctx, in.Model, anthropicFallbackModels(&in))
	} else {
		var in openai.OpenAIResponsesRequest
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, false, fmt.Errorf("invalid request body: %w", err)
		}
		req, stream = in.ToBifrostResponsesRequest(ctx), in.Stream
		req.Provider, req.Model, req.Fallbacks = e.normalizeRequestModels(ctx, in.Model, in.Fallbacks)
	}
	if req.Provider == "" {
		// Obot's proxy resolves bare model IDs before the engine sees them.
		return nil, false, fmt.Errorf("model %q must be named as provider/model", req.Model)
	}
	return req, stream != nil && *stream, nil
}

// serveStream relays the engine's stream as server-sent events in the request's format. Headers
// are written with the first encoded event, by which point retries and fallbacks are over.
// An error before that event is an ordinary JSON error, and the served model is known.
func (e *Engine) serveStream(w http.ResponseWriter, ctx *schemas.BifrostContext, req *schemas.BifrostResponsesRequest, d dialect) {
	chunks, bifrostErr := e.client.ResponsesStreamRequest(ctx, req)
	if bifrostErr != nil {
		observeError(ctx, bifrostErr)
		writeEngineError(w, d, bifrostErr)
		return
	}
	defer drainCanceledStream(ctx, chunks)

	output := streamWriter{w: w, dialect: d}
	for {
		var (
			chunk *schemas.BifrostStreamChunk
			ok    bool
		)
		// Producer cleanup may delay closure after the client disconnects.
		select {
		case <-ctx.Done():
			return
		case chunk, ok = <-chunks:
		}
		if !ok || ctx.Err() != nil {
			return
		}

		if chunk == nil {
			continue
		}
		if chunk.BifrostError != nil {
			observeError(ctx, chunk.BifrostError)
			output.fail(chunk.BifrostError)
			return
		}

		event := chunk.BifrostResponsesStreamResponse
		if event == nil {
			continue
		}
		observeResponse(ctx, event.ExtraFields)
		switch d {
		case dialectResponses:
			if out := event.WithDefaults(); out != nil {
				if err := output.write(string(out.Type), out, event.ExtraFields); err != nil {
					return
				}
			}
		case dialectMessages:
			for _, out := range anthropic.ToAnthropicResponsesStreamResponse(ctx, event) {
				if err := output.write(string(out.Type), out, event.ExtraFields); err != nil {
					return
				}
			}
		}
		e.observeStream(ctx, chunk)
	}
}

func setServed(h http.Header, extra schemas.BifrostResponseExtraFields) {
	route := extra.RoutingInfo
	if route.Provider != "" {
		h.Set(headerServedProvider, string(route.Provider))
	}
	model := route.Model
	if route.ResolvedKeyAlias != nil {
		model = route.ResolvedKeyAlias.ModelID
	}
	if model != "" {
		h.Set(headerServedModel, model)
	}
}

// marshal encodes a response without the engine's own fields.
func marshal(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("failed to encode LLM gateway response", "error", err)
		return nil, err
	}

	for _, path := range []string{"extra_fields", "response.extra_fields"} {
		if gjson.GetBytes(data, path).Exists() {
			stripped, err := sjson.DeleteBytes(data, path)
			if err != nil {
				return nil, err
			}
			data = stripped
		}
	}
	return data, nil
}

func writeJSON(w http.ResponseWriter, d dialect, status int, v any) {
	// Encode before committing headers so failures cannot become empty successes.
	data, err := marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		// The fallback body contains only fixed strings and is always encodable.
		data, _ = json.Marshal(errorBody(d, "api_error", responseEncodingMessage))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeEvent(w http.ResponseWriter, name string, v any) error {
	data, err := marshal(v)
	if err != nil {
		return err
	}
	return writeEncodedEvent(w, name, data)
}

// errorParts returns an engine error's status, type and message, without the engine's name.
func errorParts(err *schemas.BifrostError) (int, string, string) {
	// Core validation failures can omit StatusCode. Use Bifrost's classification
	// so those stay caller errors and explicit provider statuses take precedence.
	status := err.EffectiveHTTPStatus()
	if isProviderConfigGateError(err) {
		status = http.StatusServiceUnavailable
	}

	errType, message := "api_error", "the LLM gateway failed to serve the request"
	if status == http.StatusBadRequest {
		errType = "invalid_request_error"
	}
	if err.Error != nil {
		if err.Error.Type != nil && *err.Error.Type != "" {
			errType = *err.Error.Type
		}
		if err.Error.Message != "" {
			message = engineName.ReplaceAllString(err.Error.Message, "gateway")
		}
	}
	return status, errType, message
}

func writeEngineError(w http.ResponseWriter, d dialect, err *schemas.BifrostError) {
	status, errType, message := errorParts(err)
	writeError(w, d, status, errType, message)
}

func writeError(w http.ResponseWriter, d dialect, status int, errType, message string) {
	writeJSON(w, d, status, errorBody(d, errType, message))
}

func writeStreamError(w http.ResponseWriter, d dialect, err *schemas.BifrostError, sequence int) error {
	_, errType, message := errorParts(err)
	if d == dialectResponses {
		// Responses uses an event union with top-level error fields, whereas
		// Chat Completions and Messages retain their nested error envelopes.
		code := errType
		if err.Error != nil && err.Error.Code != nil && *err.Error.Code != "" {
			code = *err.Error.Code
		}
		event := &schemas.BifrostResponsesStreamResponse{
			Type:           schemas.ResponsesStreamResponseTypeError,
			SequenceNumber: sequence,
			Code:           &code,
			Message:        &message,
		}
		if err.Error != nil {
			if param, ok := err.Error.Param.(string); ok {
				event.Param = &param
			}
		}
		return writeEvent(w, "error", event)
	}
	return writeEvent(w, "error", errorBody(d, errType, message))
}

func errorBody(d dialect, errType, message string) map[string]any {
	if d == dialectMessages {
		return map[string]any{"type": "error", "error": map[string]any{"type": errType, "message": message}}
	}
	return map[string]any{"error": map[string]any{"type": errType, "message": message}}
}

// WithResponseObserver reports metadata for unary responses and every stream chunk,
// including final latency metadata. Cost and usage reporting belong to plugin hooks.
func WithResponseObserver(ctx context.Context, observe func(schemas.BifrostResponseExtraFields)) context.Context {
	return context.WithValue(ctx, responseObserverKey{}, observe)
}

// WithStreamObserver observes chunks after guardrail replay and successful client encoding.
// The accumulator belongs to the engine; observers must clean up their per-request state.
func WithStreamObserver(ctx context.Context, observe func(*schemas.BifrostContext, *schemas.BifrostStreamChunk, *streaming.Accumulator)) context.Context {
	return context.WithValue(ctx, streamObserverKey{}, observe)
}

func (e *Engine) observeStream(ctx *schemas.BifrostContext, chunk *schemas.BifrostStreamChunk) {
	if observe, ok := ctx.Value(streamObserverKey{}).(func(*schemas.BifrostContext, *schemas.BifrostStreamChunk, *streaming.Accumulator)); ok && observe != nil {
		observe(ctx, chunk, e.accumulator)
	}
}

func observeResponse(ctx context.Context, extra schemas.BifrostResponseExtraFields) {
	if observe, ok := ctx.Value(responseObserverKey{}).(func(schemas.BifrostResponseExtraFields)); ok && observe != nil {
		observe(extra)
	}
}

func (e *Engine) serveChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeBodyReadError(w, dialectChat, err)
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	var in openai.OpenAIChatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, dialectChat, http.StatusBadRequest, "invalid_request_error", "invalid request body: "+err.Error())
		return
	}

	req := in.ToBifrostChatRequest(ctx)
	req.Provider, req.Model, req.Fallbacks = e.normalizeRequestModels(ctx, in.Model, in.Fallbacks)
	if req.Provider == "" {
		writeError(w, dialectChat, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("model %q must be named as provider/model", req.Model))
		return
	}
	if in.Stream == nil || !*in.Stream {
		resp, engineErr := e.client.ChatCompletionRequest(ctx, req)
		if engineErr != nil {
			observeError(ctx, engineErr)
			writeEngineError(w, dialectChat, engineErr)
			return
		}
		observeResponse(ctx, resp.ExtraFields)
		setServed(w.Header(), resp.ExtraFields)
		writeJSON(w, dialectChat, http.StatusOK, resp)
		return
	}

	chunks, engineErr := e.client.ChatCompletionStreamRequest(ctx, req)
	if engineErr != nil {
		observeError(ctx, engineErr)
		writeEngineError(w, dialectChat, engineErr)
		return
	}
	defer drainCanceledStream(ctx, chunks)

	output := streamWriter{w: w, dialect: dialectChat}
	for {
		var (
			chunk *schemas.BifrostStreamChunk
			ok    bool
		)
		// Producer cleanup may delay closure after the client disconnects.
		select {
		case <-ctx.Done():
			return
		case chunk, ok = <-chunks:
		}
		if ctx.Err() != nil {
			return
		}
		if !ok {
			_ = output.finishChat()
			return
		}

		if chunk == nil {
			continue
		}
		if chunk.BifrostError != nil {
			observeError(ctx, chunk.BifrostError)
			output.fail(chunk.BifrostError)
			return
		}
		event := chunk.BifrostChatResponse
		if event == nil {
			continue
		}
		observeResponse(ctx, event.ExtraFields)
		if err := output.write("", event, event.ExtraFields); err != nil {
			return
		}
		e.observeStream(ctx, chunk)
	}
}

// Keep late response hooks within the lifetime of the request's bound state.
// Subscription admission remains locked until producer cleanup finishes.
func drainCanceledStream(ctx *schemas.BifrostContext, chunks <-chan *schemas.BifrostStreamChunk) {
	ctx.Cancel()
	for range chunks { //nolint:revive // Drain canceled producers so core can release stream resources.
	}
}

func observeError(ctx context.Context, err *schemas.BifrostError) {
	if err != nil {
		observeResponse(ctx, err.ExtraFields.RoutingInfo.ToExtraFields(err.ExtraFields.RequestType))
	}
}

// Only MaxBytesReader limit violations are size errors; other failures mean the
// request body could not be read (for example, an interrupted upload).
func writeBodyReadError(w http.ResponseWriter, d dialect, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeError(w, d, http.StatusRequestEntityTooLarge, "invalid_request_error", "request body too large")
		return
	}
	writeError(w, d, http.StatusBadRequest, "invalid_request_error", "could not read request body")
}

// ChatCompletion is for trusted internal callers that consume a complete result.
// It shares HTTP model normalization and the configured routing/plugin chain.
func (e *Engine) ChatCompletion(parent context.Context, model string, messages []schemas.ChatMessage) (resp *schemas.BifrostChatResponse, err error) {
	// Policy workers have no HTTP recovery. Contain synchronous hook failures here
	// and discard partial results without including potentially secret panic values.
	defer func() {
		if recover() != nil {
			resp = nil
			err = errors.New("LLM gateway inference failed")
		}
	}()
	ctx, cancel := e.inferenceContext(parent)
	defer cancel()
	// Preserve the routing header seen by the former internal JSON caller.
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{"content-type": "application/json"})
	provider, normalized, _ := e.normalizeRequestModels(ctx, model, nil)
	if provider == "" {
		return nil, fmt.Errorf("model %q must be named as provider/model", model)
	}
	resp, engineErr := e.client.ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{Provider: provider, Model: normalized, Input: messages})
	if engineErr != nil {
		_, _, message := errorParts(engineErr)
		return nil, errors.New(message)
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}
