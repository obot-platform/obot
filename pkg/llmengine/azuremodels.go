package llmengine

import (
	"cmp"
	"encoding/json"
	"slices"

	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// ProviderExtra carries this through Bifrost without adding internal metadata to public JSON.
type azureDeploymentMetadata struct {
	BaseModel string `json:"azure_base_model"`
}

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

		var baseModels map[string]string
		snapshot, ok := cfg.AzureDiscoveries[provider][key.ID]
		account := cfg.AzureAccounts[provider][key.ID]
		if ok && account != (llmcoreclient.AzureAccount{}) && snapshot.Fingerprint == llmcoreclient.AzureDiscoveryFingerprint(key, account) {
			baseModels = snapshot.BaseModels
		}

		entries := pipeline.BackfillModels(map[string]bool{})
		if len(discovered) > 0 {
			if names, automatic := discovered[0][key.ID]; automatic {
				entries = nil

				for _, name := range names {
					for _, match := range pipeline.FilterModel(name) {
						// Match inference's alias precedence before advertising this deployment's metadata.
						if key.Aliases.Resolve(match.ResolvedID) != name {
							continue
						}

						model := schemas.Model{ID: string(provider) + "/" + match.ResolvedID}

						// Deduplication must keep metadata from the same eligible key that advertised the ID.
						if base := baseModels[name]; base != "" {
							model.ProviderExtra, _ = json.Marshal(azureDeploymentMetadata{BaseModel: base})
						}

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

func azureBaseModel(model schemas.Model) string {
	if len(model.ProviderExtra) == 0 {
		return ""
	}

	var metadata azureDeploymentMetadata
	_ = json.Unmarshal(model.ProviderExtra, &metadata)
	return metadata.BaseModel
}

func isAzureProvider(cfg llmcoreclient.Config, provider schemas.ModelProvider) bool {
	pc, exists := cfg.Providers[provider]

	return exists && (provider == schemas.Azure || (pc.CustomProviderConfig != nil && pc.CustomProviderConfig.BaseProviderType == schemas.Azure))
}
