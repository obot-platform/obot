package llmcore

import (
	"net/http"

	"github.com/maximhq/bifrost/core/schemas"
)

type failClosedPostHook struct {
	schemas.LLMPlugin
	logger schemas.Logger
}

func (p failClosedPostHook) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (result *schemas.BifrostResponse, resultErr *schemas.BifrostError, hookErr error) {
	// Bifrost's own recovery passes the original provider response onward. A
	// panicking output guardrail could therefore let unchecked content reach
	// the caller. Discard the response being checked and fail the request.
	// For a stream, chunks already sent to the caller cannot be recalled.
	defer func() {
		if recover() != nil {
			p.logger.Error("LLM post-hook panicked for plugin %s", p.GetName())
			result = nil
			resultErr = &schemas.BifrostError{
				StatusCode:     new(http.StatusInternalServerError),
				AllowFallbacks: new(false),
				Error: &schemas.ErrorField{
					Type:    new("internal_error"),
					Message: "LLM gateway inference failed",
				},
			}
		}
	}()
	return p.LLMPlugin.PostLLMHook(ctx, resp, bifrostErr)
}
