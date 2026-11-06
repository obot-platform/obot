package llmcore

import (
	"context"
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llm"
)

// requestContext forwards compatibility headers and privately retains an admitted
// subscription credential. Only per-attempt admission can add it to outgoing headers.
func (e *Engine) requestContext(r *http.Request) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := e.inferenceContext(r.Context())
	if credential, _ := r.Context().Value(subscriptionTokenKey{}).(subscriptionCredentialValue); credential.token != "" {
		ctx.SetValue(subscriptionTokenKey{}, credential)
	}

	headers := make(map[string]string, len(r.Header))
	extra := map[string][]string{}
	for name := range r.Header {
		lower := strings.ToLower(name)
		if isCredentialHeader(lower) || lower == "chatgpt-account-id" || lower == "x-obot-api-key" {
			continue
		}
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
	// repeated values. Subscription admission adds Authorization per attempt.
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	// Bifrost uses the client identity to select provider compatibility behavior.
	ctx.SetValue(schemas.BifrostContextKeyUserAgent, r.UserAgent())
	return ctx, cancel
}

// inferenceContext gives HTTP and trusted native calls the same model snapshot,
// catalog and plugin context, without manufacturing an internal HTTP request.
func (e *Engine) inferenceContext(parent context.Context) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(parent)
	// A judge can run inside another inference call. Do not inherit its subscription
	// credential or selection overrides; HTTP explicitly hands off its own token above.
	ctx.SetValue(subscriptionTokenKey{}, subscriptionCredentialValue{})
	ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, false)
	ctx.SetValue(schemas.BifrostContextKeyDirectKey, nil)
	// Nested calls select their own credentials; their routing rules can set a new pin.
	ctx.SetValue(schemas.BifrostContextKeyRoutingPinnedAPIKeyID, "")
	ctx.SetValue(schemas.BifrostContextKeyAPIKeyID, "")
	ctx.SetValue(schemas.BifrostContextKeyAPIKeyName, "")
	ctx.SetValue(schemas.BifrostContextKeyURLPath, nil)
	llm.SetSubscriptionAttempt(ctx, false)

	// Parsing uses one replica-local view for primary, caller and rule fallbacks.
	ctx.SetValue(modelConfigKey{}, e.account.config.Load())
	if e.catalog != nil {
		ctx.SetValue(schemas.BifrostContextKeyModelCatalog, e.catalog)
	}

	ctx.SetValue(schemas.BifrostContextKeyRequestQuery, map[string]string{})
	ctx.SetValue(schemas.BifrostContextKeyRequestHeaders, map[string]string{})
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{})
	return ctx, cancel
}
