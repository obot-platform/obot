package llmcore

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/maximhq/bifrost/core/schemas"
)

// requestContext translates only headers already admitted by Obot's authenticated proxy.
// The engine relies on stripCallerCredentials in pkg/gateway/server/llmproxy_engine.go
// to remove Obot credentials and API keys, admitting only admin-approved subscription
// tokens for a verified Anthropic destination. credentialPlugin enforces the
// Anthropic boundary again after routing and on each fallback attempt.
// Only ClaudeCLI subscription mode is supported; the attempt hook refuses other
// credential-bearing modes before a worker can select a configured key.
func (e *Engine) requestContext(r *http.Request) (*schemas.BifrostContext, context.CancelFunc) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(r.Context())
	// Parsing uses one replica-local view for primary, caller and rule fallbacks.
	// Credential safety must instead read the current account on every attempt.
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
		case "anthropic-beta", "anthropic-version", "authorization", "user-agent", "x-api-key", "x-stainless-lang", "x-stainless-package-version", "x-stainless-os", "x-stainless-arch", "x-stainless-runtime", "x-stainless-runtime-version", "x-stainless-retry-count", "x-stainless-timeout":
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
	// repeated values. credentialPlugin restricts caller credentials per attempt.
	ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, extra)
	// Bifrost uses the client identity to select compatibility behavior, including
	// Anthropic's Claude Code subscription handling.
	ctx.SetValue(schemas.BifrostContextKeyUserAgent, r.UserAgent())
	if hasCallerCredentials(extra) {
		// Every credential-bearing mode must reach the hook without using Obot's
		// key pool. The hook admits only supported ClaudeCLI subscription mode.
		ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
	}
	// TryRLock refuses new subscription admissions while retirement owns or
	// waits for the write lease. Waiting here would strand canceled HTTP requests
	// behind an unrelated configured-key request that is still draining.
	if supportedSubscription(ctx, extra) {
		lease := &subscriptionLease{account: e.account, owner: ctx.Root()}
		if e.account.subscriptionMu.TryRLock() {
			lease.state.Store(subscriptionLeaseActive)
		} else {
			// Only requestContext can record an HTTP admission refused by retirement.
			// This marker permits a retryable error, never credential forwarding.
			lease.state.Store(subscriptionLeasePending)
		}
		ctx.SetValue(subscriptionLeaseKey{}, lease)
		// The handler defers this owned cancel across every return path. Parent
		// cancellation signals core, but cannot release the lease while hooks or
		// stream cleanup are still returning to the handler.
		var once sync.Once
		cancelContext := cancel
		cancel = func() {
			once.Do(func() {
				cancelContext()
				if lease.state.Swap(0) == subscriptionLeaseActive {
					e.account.subscriptionMu.RUnlock()
				}
			})
		}
	}
	return ctx, cancel
}

type subscriptionLeaseKey struct{}

const (
	subscriptionLeasePending uint32 = 1
	subscriptionLeaseActive  uint32 = 2
)

type subscriptionLease struct {
	account *account
	// Private context values are inherited by derived Bifrost roots. Only this
	// HTTP root (including its plugin scopes) is covered by the handler's lease.
	owner *schemas.BifrostContext
	state atomic.Uint32 // zero after the HTTP owner cancels
}
