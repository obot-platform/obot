package llmcore

import (
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// normalizeRequestModels replaces converter parsing with one replica-local view.
// Retirement unregisters custom names globally, even when this account still
// retains them as pending providers. Admission and credentials remain per attempt.
func (e *Engine) normalizeRequestModels(model string, fallbacks []string) (schemas.ModelProvider, string, []schemas.Fallback) {
	cfg := e.account.config.Load()
	provider, model := parseConfiguredModel(cfg, model)
	if len(fallbacks) == 0 {
		return provider, model, nil
	}

	parsed := make([]schemas.Fallback, 0, len(fallbacks))
	for _, fallback := range fallbacks {
		fallbackProvider, fallbackModel := parseConfiguredModel(cfg, fallback)
		if fallbackProvider != "" && fallbackModel != "" {
			parsed = append(parsed, schemas.Fallback{Provider: fallbackProvider, Model: fallbackModel})
		}
	}

	return provider, model, parsed
}

func parseConfiguredModel(cfg *llmcoreclient.Config, model string) (schemas.ModelProvider, string) {
	prefix, suffix, qualified := strings.Cut(model, "/")
	if qualified && prefix != "" {
		provider := schemas.ModelProvider(prefix)
		_, configured := cfg.Providers[provider]
		if configured || slices.Contains(schemas.StandardProviders, provider) {
			return provider, suffix
		}
	}

	// Unknown namespaces stay part of the model; HTTP still requires a provider.
	return "", model
}

func anthropicFallbackModels(in *anthropic.AnthropicMessageRequest) []string {
	if in.Fallbacks == nil {
		return nil
	}

	// Anthropic also accepts native fallback objects/presets. Only string entries
	// describe Bifrost cross-provider attempts; preserve native conversion separately.
	var models []string
	for _, fallback := range in.Fallbacks.Entries {
		if fallback.Native == nil {
			models = append(models, fallback.BifrostModel)
		}
	}

	return models
}
