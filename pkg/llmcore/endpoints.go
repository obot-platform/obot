package llmcore

import (
	"io"
	"net/http"

	bifrost "github.com/maximhq/bifrost/core"
)

// Client exposes the trusted in-process core API. Public requests must use the
// authenticated proxy, which admits subscriptions and strips other caller credentials.
func (e *Engine) Client() *bifrost.Bifrost { return e.client }

func (e *Engine) serveCount(w http.ResponseWriter, r *http.Request, d dialect) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, e.MaxRequestBodyBytes()))
	if err != nil {
		writeBodyReadError(w, d, err)
		return
	}

	ctx, cancel := e.requestContext(r)
	defer cancel()
	req, _, err := e.decodeResponsesRequest(ctx, body, d)
	if err != nil {
		writeError(w, d, http.StatusBadRequest, "invalid_request_error", "invalid token counting request")
		return
	}

	out, engineErr := e.client.CountTokensRequest(ctx, req)
	if engineErr != nil {
		observeError(ctx, engineErr)
		writeEngineError(w, d, engineErr)
		return
	}
	observeResponse(ctx, out.ExtraFields)
	setServed(w.Header(), out.ExtraFields)

	if d == dialectMessages {
		writeJSON(w, d, http.StatusOK, map[string]any{"input_tokens": out.InputTokens})
	} else {
		writeJSON(w, d, http.StatusOK, out)
	}
}
