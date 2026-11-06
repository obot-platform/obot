package llmcore

import (
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/providers/anthropic"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

type modelConfigKey struct{}

// normalizeRequestModels replaces converter parsing with one replica-local view.
// Retirement unregisters custom names globally, even when this account still
// retains them as pending providers. Admission and credentials remain per attempt.
func (e *Engine) normalizeRequestModels(ctx *schemas.BifrostContext, model string, fallbacks []string) (schemas.ModelProvider, string, []schemas.Fallback) {
	cfg := requestModelConfig(ctx, e.account)
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
		configured := false
		if cfg != nil {
			_, configured = cfg.Providers[provider]
		}
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

// Direct Client callers have no HTTP parsing snapshot. Use this replica's
// account rather than Bifrost's process-global custom-provider registry.
func requestModelConfig(ctx *schemas.BifrostContext, a *account) *llmcoreclient.Config {
	if cfg, ok := ctx.Value(modelConfigKey{}).(*llmcoreclient.Config); ok {
		return cfg
	}
	if a != nil {
		return a.config.Load()
	}
	return nil
}

func resolveConfiguredRuleFallback(cfg *llmcoreclient.Config, fb tables.RoutingFallback) schemas.Fallback {
	resolved := fb.Fallback
	// String prefers the private raw legacy spelling over exported fields. Clear
	// those fields on a copy to distinguish legacy strings from explicit objects,
	// whose providers (including missing ones) and key pins must stay unchanged.
	legacy := fb
	legacy.Fallback = schemas.Fallback{}
	raw := legacy.String()
	// Pinning a legacy string can persist it as an object with no provider and
	// the full spelling in Model. Resolve that representation without losing KeyID.
	if raw == "" && resolved.Provider == "" {
		raw = resolved.Model
	}
	if raw != "" {
		resolved.Provider, resolved.Model = parseConfiguredModel(cfg, raw)
	}
	return resolved
}
