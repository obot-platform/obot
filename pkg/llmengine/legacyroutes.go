package llmengine

import "context"

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
	source, _ := ctx.Value(legacySourceKey{}).(string)
	keys, err := e.ListProviderKeys(ctx, provider)
	if err != nil {
		return "", err
	}
	for _, key := range keys {
		if key.Name == "imported-"+source {
			return key.ID, nil
		}
	}
	return "", nil
}
