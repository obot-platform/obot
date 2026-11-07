package llmengine

import (
	"cmp"
	"slices"

	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// Azure's data-plane endpoint lists offerings; use ARM discoveries or explicitly configured names.
func azureDeploymentModels(cfg llmcoreclient.Config, provider schemas.ModelProvider, discovered ...map[string][]string) ([]schemas.Model, bool) {
	if !isAzureProvider(cfg, provider) {
		return nil, false
	}

	models := []schemas.Model{}
	seen := map[string]bool{}

	for _, key := range cfg.Keys[provider] {
		if key.Enabled != nil && !*key.Enabled {
			continue
		}

		pipeline := providerUtils.ListModelsPipeline{
			AllowedModels: key.Models, BlacklistedModels: key.BlacklistedModels,
			Aliases: key.Aliases, ProviderKey: provider, MatchFns: providerUtils.DefaultMatchFns(),
		}

		if pipeline.ShouldEarlyExit() {
			continue
		}

		entries := pipeline.BackfillModels(map[string]bool{})
		if len(discovered) > 0 {
			if names, automatic := discovered[0][key.ID]; automatic {
				entries = nil

				for _, name := range names {
					for _, match := range pipeline.FilterModel(name) {
						model := schemas.Model{ID: string(provider) + "/" + match.ResolvedID}
						if match.AliasValue != "" {
							model.Alias = new(match.AliasValue)
						}

						entries = append(entries, model)
					}
				}
			}
		}

		for _, model := range entries {
			if !seen[model.ID] {
				models = append(models, model)
				seen[model.ID] = true
			}
		}
	}

	slices.SortFunc(models, func(a, b schemas.Model) int { return cmp.Compare(a.ID, b.ID) })

	return models, true
}

func isAzureProvider(cfg llmcoreclient.Config, provider schemas.ModelProvider) bool {
	pc, exists := cfg.Providers[provider]

	return exists && (provider == schemas.Azure || (pc.CustomProviderConfig != nil && pc.CustomProviderConfig.BaseProviderType == schemas.Azure))
}
