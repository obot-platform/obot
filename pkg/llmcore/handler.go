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

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// headerServedProvider and headerServedModel name the provider and model that served a
	// response, which routing rules and fallbacks can make differ from the requested ones.
	headerServedProvider = "X-Obot-LLM-Provider"
	headerServedModel    = "X-Obot-LLM-Model"

	maxRequestBodyBytes         = 100 << 20
	dialectResponses    dialect = 0
	dialectMessages     dialect = 1
)

// engineName matches a library error prefix, preserving provider text that merely mentions it.
var (
	engineName = regexp.MustCompile(`(?i)^bifrost\b`)
)

type dialect int
type responseObserverKey struct{}

// ServeHTTP serves the OpenAI Responses API at /v1/responses and the Anthropic Messages API at
// /v1/messages (and /anthropic/v1/messages, the path Obot's proxy uses for the engine today).
// Either API can reach any provider's models: the engine translates.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	var (
		req    *schemas.BifrostResponsesRequest
		stream bool
	)
	switch d {
	case dialectResponses:
		var in openai.OpenAIResponsesRequest
		if err := json.Unmarshal(body, &in); err != nil {
			writeError(w, d, http.StatusBadRequest, "invalid_request_error", "invalid request body: "+err.Error())
			return
		}
		req, stream = in.ToBifrostResponsesRequest(ctx), in.Stream != nil && *in.Stream
	case dialectMessages:
		var in anthropic.AnthropicMessageRequest
		if err := json.Unmarshal(body, &in); err != nil {
			writeError(w, d, http.StatusBadRequest, "invalid_request_error", "invalid request body: "+err.Error())
			return
		}
		req, stream = in.ToBifrostResponsesRequest(ctx), in.Stream != nil && *in.Stream
	}
	if req.Provider == "" {
		// Obot's proxy resolves bare model IDs to provider/model before the engine sees them.
		writeError(w, d, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("model %q must be named as provider/model", req.Model))
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
		writeJSON(w, http.StatusOK, resp.WithDefaults())
	case dialectMessages:
		writeJSON(w, http.StatusOK, anthropic.ToAnthropicResponsesResponse(ctx, resp))
	}
}

// serveStream relays the engine's stream as server-sent events in the request's format. Headers
// are written with the first chunk, by which point retries and fallbacks are over, so an error
// before the first chunk is an ordinary JSON error, and the served model is known.
func (e *Engine) serveStream(w http.ResponseWriter, ctx *schemas.BifrostContext, req *schemas.BifrostResponsesRequest, d dialect) {
	chunks, bifrostErr := e.client.ResponsesStreamRequest(ctx, req)
	if bifrostErr != nil {
		observeError(ctx, bifrostErr)
		writeEngineError(w, d, bifrostErr)
		return
	}
	defer func() {
		// Stop the engine and let it close the stream if this returns early.
		ctx.Cancel()
		for range chunks { //nolint:revive // Drain canceled producers so core can release stream resources.
		}
	}()

	started := false
	for chunk := range chunks {
		if chunk == nil {
			continue
		}
		if chunk.BifrostError != nil {
			if !started {
				observeError(ctx, chunk.BifrostError)
				writeEngineError(w, d, chunk.BifrostError)
				return
			}
			observeError(ctx, chunk.BifrostError)
			writeStreamError(w, d, chunk.BifrostError)
			return
		}

		event := chunk.BifrostResponsesStreamResponse
		if event == nil {
			continue
		}
		observeResponse(ctx, event.ExtraFields)
		if !started {
			setServed(w.Header(), event.ExtraFields)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			started = true
		}

		switch d {
		case dialectResponses:
			if out := event.WithDefaults(); out != nil {
				if err := writeEvent(w, string(out.Type), out); err != nil {
					return
				}
			}
		case dialectMessages:
			for _, out := range anthropic.ToAnthropicResponsesStreamResponse(ctx, event) {
				if err := writeEvent(w, string(out.Type), out); err != nil {
					return
				}
			}
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	// Encode before committing headers so failures cannot become empty successes.
	data, err := marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"error":{"type":"api_error","message":"failed to encode gateway response"}}`)
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
	if name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

// errorParts returns an engine error's status, type and message, without the engine's name.
func errorParts(err *schemas.BifrostError) (int, string, string) {
	status := http.StatusInternalServerError
	if err.StatusCode != nil && *err.StatusCode >= http.StatusBadRequest {
		status = *err.StatusCode
	}

	errType, message := "api_error", "the LLM gateway failed to serve the request"
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
	writeJSON(w, status, errorBody(d, errType, message))
}

func writeStreamError(w http.ResponseWriter, d dialect, err *schemas.BifrostError) {
	_, errType, message := errorParts(err)
	_ = writeEvent(w, "error", errorBody(d, errType, message))
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

func observeResponse(ctx context.Context, extra schemas.BifrostResponseExtraFields) {
	if observe, ok := ctx.Value(responseObserverKey{}).(func(schemas.BifrostResponseExtraFields)); ok && observe != nil {
		observe(extra)
	}
}

func (e *Engine) serveChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeBodyReadError(w, dialectResponses, err)
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	var in openai.OpenAIChatRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, dialectResponses, http.StatusBadRequest, "invalid_request_error", "invalid request body: "+err.Error())
		return
	}

	req := in.ToBifrostChatRequest(ctx)
	if req.Provider == "" {
		writeError(w, dialectResponses, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("model %q must be named as provider/model", req.Model))
		return
	}
	if in.Stream == nil || !*in.Stream {
		resp, engineErr := e.client.ChatCompletionRequest(ctx, req)
		if engineErr != nil {
			observeError(ctx, engineErr)
			writeEngineError(w, dialectResponses, engineErr)
			return
		}
		observeResponse(ctx, resp.ExtraFields)
		setServed(w.Header(), resp.ExtraFields)
		writeJSON(w, http.StatusOK, resp)
		return
	}

	chunks, engineErr := e.client.ChatCompletionStreamRequest(ctx, req)
	if engineErr != nil {
		observeError(ctx, engineErr)
		writeEngineError(w, dialectResponses, engineErr)
		return
	}
	defer func() {
		ctx.Cancel()
		for range chunks { //nolint:revive // Drain canceled producers so core can release stream resources.
		}
	}()

	started := false
	for chunk := range chunks {
		if chunk == nil {
			continue
		}

		if chunk.BifrostError != nil {
			if !started {
				observeError(ctx, chunk.BifrostError)
				writeEngineError(w, dialectResponses, chunk.BifrostError)
			} else {
				observeError(ctx, chunk.BifrostError)
				writeStreamError(w, dialectResponses, chunk.BifrostError)
			}
			return
		}

		event := chunk.BifrostChatResponse
		if event == nil {
			continue
		}

		observeResponse(ctx, event.ExtraFields)
		if !started {
			setServed(w.Header(), event.ExtraFields)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			started = true
		}

		if err := writeEvent(w, "", event); err != nil {
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
	}
	if started {
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		_ = http.NewResponseController(w).Flush()
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
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, d, http.StatusRequestEntityTooLarge, "invalid_request_error", "request body too large")
		return
	}
	writeError(w, d, http.StatusBadRequest, "invalid_request_error", "could not read request body")
}
