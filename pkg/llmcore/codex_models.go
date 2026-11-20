package llmcore

import (
	"net/http"
	"net/url"

	"github.com/maximhq/bifrost/core/schemas"
)

// Native model metadata includes instructions and capabilities absent from the
// OpenAI API model list. Keep its raw body inside core's normal hook/worker lifecycle.
func (e *Engine) serveCodexModels(w http.ResponseWriter, r *http.Request) {
	credential, _ := r.Context().Value(subscriptionTokenKey{}).(subscriptionCredentialValue)
	if credential.provider != schemas.OpenAI {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := e.requestContext(r)
	defer cancel()

	// Only the version parameter is part of catalog discovery; caller routing or
	// endpoint overrides must not become upstream query parameters.
	query := url.Values{}
	if versions, ok := r.URL.Query()["client_version"]; ok {
		query["client_version"] = versions
	}
	resp, err := e.client.Passthrough(ctx, schemas.OpenAI, &schemas.BifrostPassthroughRequest{
		Method:      http.MethodGet,
		Path:        "/models",
		UpstreamURL: codexUpstream,
		RawQuery:    query.Encode(),
	})
	if err != nil {
		observeError(ctx, err)
		writeEngineError(w, dialectResponses, err)
		return
	}
	observeResponse(ctx, resp.ExtraFields)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}
