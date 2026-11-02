package llmcore

import (
	"encoding/json"
	"io"
	"net/http"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/providers/openai"
	"github.com/maximhq/bifrost/core/schemas"
)

func (e *Engine) Client() *bifrost.Bifrost { return e.client }

func (e *Engine) serveCount(w http.ResponseWriter, r *http.Request, d dialect) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeError(w, d, 413, "invalid_request_error", "request body too large")
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	var req *schemas.BifrostResponsesRequest
	if d == dialectMessages {
		var in anthropic.AnthropicMessageRequest
		err = json.Unmarshal(body, &in)
		if err == nil {
			req = in.ToBifrostResponsesRequest(ctx)
		}
	} else {
		var in openai.OpenAIResponsesRequest
		err = json.Unmarshal(body, &in)
		if err == nil {
			req = in.ToBifrostResponsesRequest(ctx)
		}
	}
	if err != nil || req == nil || req.Provider == "" {
		writeError(w, d, 400, "invalid_request_error", "invalid token counting request")
		return
	}

	out, engineErr := e.client.CountTokensRequest(ctx, req)
	if engineErr != nil {
		observeError(ctx, engineErr)
		writeEngineError(w, d, engineErr)
		return
	}

	if d == dialectMessages {
		writeJSON(w, 200, map[string]any{"input_tokens": out.InputTokens})
	} else {
		writeJSON(w, 200, out)
	}
}

func (e *Engine) serveEmbedding(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeError(w, dialectResponses, 413, "invalid_request_error", "request body too large")
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	var in openai.OpenAIEmbeddingRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, dialectResponses, 400, "invalid_request_error", "invalid embedding request")
		return
	}

	req := in.ToBifrostEmbeddingRequest(ctx)
	if req.Provider == "" {
		writeError(w, dialectResponses, 400, "invalid_request_error", "model must be named as provider/model")
		return
	}

	out, engineErr := e.client.EmbeddingRequest(ctx, req)
	if engineErr != nil {
		observeError(ctx, engineErr)
		writeEngineError(w, dialectResponses, engineErr)
		return
	}
	observeResponse(ctx, out.ExtraFields)
	setServed(w.Header(), out.ExtraFields)
	writeJSON(w, 200, out)
}
