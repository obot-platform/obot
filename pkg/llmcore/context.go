package llmcore

import (
	"context"
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// requestContext translates only headers already admitted by Obot's authenticated proxy.
// stripCallerCredentials removes Obot credentials and API keys; only admin-approved
// Anthropic subscription tokens for a verified Anthropic destination survive.
// The User-Agent check below selects Bifrost compatibility behavior, not authorization.
func (e *Engine) requestContext(r *http.Request) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(r.Context())
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
		case "anthropic-beta", "anthropic-version", "authorization", "user-agent", "x-api-key", "x-stainless-lang", "x-stainless-package-version", "x-stainless-os", "x-stainless-arch", "x-stainless-runtime", "x-stainless-runtime-version", "x-stainless-retry-count", "x-stainless-timeout":
			extra[lower] = append([]string(nil), r.Header.Values(name)...)
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
	ctx.SetValue(schemas.BifrostContextKeyRequestQuery, query)
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, headers)
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	ctx.SetValue(schemas.BifrostContextKeyUserAgent, r.UserAgent())
	if schemas.ClaudeCLI.Matches(r.UserAgent()) && r.Header.Get("Authorization") != "" {
		// Claude Code supplies its own Authorization credential. Skip the configured
		// key pool so Bifrost forwards that credential instead of selecting an Obot key.
		ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
	}
	return ctx, cancel
}
