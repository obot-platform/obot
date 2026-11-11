package llmengine

import (
	"context"

	"github.com/maximhq/bifrost/core/schemas"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type legacySourceKey struct{}

// WithLegacySource retains the old credential family even when providers were merged.
func WithLegacySource(ctx context.Context, source string) context.Context {
	return context.WithValue(ctx, legacySourceKey{}, source)
}

func LegacyProvider(ctx context.Context) string {
	source, _ := ctx.Value(legacySourceKey{}).(string)
	return legacyImportPlans[source].provider
}

// LegacyKey prefers the imported credential; new configurations use the normal key pool.
func (e *Engine) LegacyKey(ctx context.Context, provider string) (string, error) {
	if !e.Ready() {
		return "", apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	source, _ := ctx.Value(legacySourceKey{}).(string)
	key, configured := e.core.Load().ProviderKeyID(schemas.ModelProvider(provider), "imported-"+source)
	if !configured {
		return "", missing("provider")
	}
	return key, nil
}
