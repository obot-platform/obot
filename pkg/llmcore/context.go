package llmcore

import (
	"context"
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// requestContext forwards compatibility headers after the authenticated proxy has
// removed caller credentials. Inference uses provider configuration for authentication.
func (e *Engine) requestContext(r *http.Request) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(r.Context())
	// Parsing uses one replica-local view for primary, caller and rule fallbacks.
	ctx.SetValue(modelConfigKey{}, e.account.config.Load())
	if e.catalog != nil {
		ctx.SetValue(schemas.BifrostContextKeyModelCatalog, e.catalog)
	}

	headers := make(map[string]string, len(r.Header))
	extra := map[string][]string{}
	for name := range r.Header {
		lower := strings.ToLower(name)
		value := r.Header.Get(name)
		headers[lower] = value
		switch lower {
		case "anthropic-beta", "anthropic-version", "user-agent", "x-stainless-lang", "x-stainless-package-version", "x-stainless-os", "x-stainless-arch", "x-stainless-runtime", "x-stainless-runtime-version", "x-stainless-retry-count", "x-stainless-timeout":
			extra[lower] = append(extra[lower], r.Header[name]...)
		}
	}

	// Match the transport's params map: lowercase names, with the last value
	// of a repeated parameter available to CEL routing expressions.
	query := make(map[string]string)
	for name, values := range r.URL.Query() {
		if len(values) > 0 {
			query[strings.ToLower(name)] = values[len(values)-1]
		}
	}
	// Routing exposes URL query parameters through CEL's params variable.
	ctx.SetValue(schemas.BifrostContextKeyRequestQuery, query)
	// Routing exposes normalized incoming headers through CEL's headers variable.
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, headers)
	// Provider adapters forward this allowlist as upstream HTTP headers, preserving
	// repeated values. Caller credentials are never forwarded.
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	// Bifrost uses the client identity to select provider compatibility behavior.
	ctx.SetValue(schemas.BifrostContextKeyUserAgent, r.UserAgent())
	return ctx, cancel
}
