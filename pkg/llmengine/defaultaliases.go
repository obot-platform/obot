package llmengine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// defaultModelAliasTargets are the gateway models default model aliases are set to when
	// unset, in order of preference.
	defaultModelAliasTargets = map[types.DefaultModelAliasType][]string{
		types.DefaultModelAliasTypeLLM:             {"openai/gpt-5.4", "anthropic/claude-sonnet-4-6"},
		types.DefaultModelAliasTypeLLMMini:         {"openai/gpt-5-mini", "anthropic/claude-haiku-4-5"},
		types.DefaultModelAliasTypeVision:          {"openai/gpt-5.4", "anthropic/claude-sonnet-4-6"},
		types.DefaultModelAliasTypeImageGeneration: {"openai/dall-e-3"},
		types.DefaultModelAliasTypeTextEmbedding:   {"openai/text-embedding-3-large"},
	}
)

// PreferredModels returns the gateway models (provider/model) preferred for a default model
// alias, most preferred first.
func PreferredModels(alias types.DefaultModelAliasType) []string {
	return defaultModelAliasTargets[alias]
}

// FillDefaultModelAliases sets unset default model aliases to the most preferred gateway
// model that is available, so a new installation works as soon as a provider is added.
func FillDefaultModelAliases(ctx context.Context, c kclient.Client) error {
	var aliases v1.DefaultModelAliasList
	if err := c.List(ctx, &aliases, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}
	var models v1.ModelList
	if err := c.List(ctx, &models, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{"spec.manifest.modelProvider": system.LLMGatewayModelProvider}); err != nil {
		return err
	}
	byTarget := map[string]string{}
	for _, m := range models.Items {
		if m.Spec.Manifest.Active {
			byTarget[m.Spec.Manifest.TargetModel] = m.Name
		}
	}
	for i := range aliases.Items {
		a := &aliases.Items[i]
		if a.Spec.Manifest.Model != "" {
			continue
		}
		for _, target := range defaultModelAliasTargets[types.DefaultModelAliasType(a.Spec.Manifest.Alias)] {
			name, ok := byTarget[target]
			if !ok {
				continue
			}
			a.Spec.Manifest.Model = name
			if err := c.Update(ctx, a); err != nil {
				return fmt.Errorf("failed to set default model alias %s: %w", a.Name, err)
			}
			slog.Info("set a default model alias to an LLM gateway model", "alias", a.Spec.Manifest.Alias, "model", target)
			break
		}
	}
	return nil
}
