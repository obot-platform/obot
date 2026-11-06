package llm

import (
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
