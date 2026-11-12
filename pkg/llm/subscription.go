package llm

import (
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

type subscriptionAttemptKey struct{}

// SetSubscriptionAttempt classifies an engine-admitted attempt before access and
// budget hooks. Only trusted engine code may set this marker; it carries no token.
func SetSubscriptionAttempt(ctx *schemas.BifrostContext, admitted bool) {
	ctx.SetValue(subscriptionAttemptKey{}, admitted)
}

// SubscriptionAttempt distinguishes an explicit engine decision from legacy
// callers that have not classified an attempt.
func SubscriptionAttempt(ctx *schemas.BifrostContext) (admitted, classified bool) {
	admitted, classified = ctx.Value(subscriptionAttemptKey{}).(bool)
	return admitted, classified
}

// IsClaudeSubscription recognizes the Anthropic OAuth credential mode at the proxy boundary.
// Core validates the CLI identity; recognizing the prefix without a User-Agent check
// keeps unsupported clients from silently spending configured provider keys.
func IsClaudeSubscription(headers http.Header) bool {
	return strings.HasPrefix(strings.ToLower(headers.Get("Authorization")), "bearer sk-ant-oat")
}

// IsCodexSubscription recognizes the CLI credential mode at the proxy boundary.
// Core validates the full credential before inference; no JWT claims are trusted.
func IsCodexSubscription(headers http.Header) bool {
	return schemas.CodexCLI.Matches(strings.ReplaceAll(headers.Get("User-Agent"), "_", "-")) &&
		strings.HasPrefix(strings.ToLower(headers.Get("Authorization")), "bearer eyj")
}
