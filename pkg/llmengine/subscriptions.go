package llmengine

import (
	"context"

	"github.com/maximhq/bifrost/core/schemas"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// SubscriptionPassthrough reads the owning aggregate for each admission. A removed or
// unreadable provider must never inherit a previously enabled value from a cache.
func (e *Engine) SubscriptionPassthrough(ctx context.Context, name string) (bool, error) {
	if !e.Ready() {
		return false, apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	if name != string(schemas.Anthropic) && name != string(schemas.OpenAI) {
		return false, nil
	}
	provider, err := e.store.GetProvider(ctx, name)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return provider.Config.CustomProviderConfig == nil && provider.SubscriptionPassthrough != nil && *provider.SubscriptionPassthrough, nil
}
