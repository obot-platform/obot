package llmengine

import (
	"context"
	"log/slog"
	"strings"

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

// LegacyKey prefers an eligible imported credential; other requests use the normal key pool.
func (e *Engine) LegacyKey(ctx context.Context, target string) (string, error) {
	if !e.Ready() {
		return "", apierrors.NewServiceUnavailable("LLM gateway unavailable")
	}
	source, _ := ctx.Value(legacySourceKey{}).(string)
	provider := LegacyProvider(ctx)
	model := strings.TrimPrefix(target, provider+"/")
	key, configured, err := e.core.Load().ProviderKeyID(ctx, schemas.ModelProvider(provider), "imported-"+source, model)
	if err != nil {
		// Key preference must not prevent normal routing, fallbacks, or provider transition gating.
		slog.WarnContext(ctx, "failed to select imported LLM gateway key; continuing without a preference", "provider", provider, "error", err)
		return "", nil
	}
	if !configured {
		return "", missing("provider")
	}
	return key, nil
}
