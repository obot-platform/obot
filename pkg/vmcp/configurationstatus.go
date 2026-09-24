package vmcp

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/utils"
)

// MissingRequiredConfiguration checks either administrator inputs or user inputs.
// Catalog literals and secret bindings are already supplied by the definition.
// Returned keys are component-scoped, so identical keys never become ambiguous.
func MissingRequiredConfiguration(component types.VMCPComponent, values map[string]string, userInputs bool) []string {
	policies := make(map[string]types.VMCPConfigurationPolicyType, len(component.Configuration))
	for _, policy := range component.Configuration {
		policies[policy.Key] = policy.Policy
	}
	var missing []string
	check := func(item types.MCPConfig) {
		if !item.Required || item.Value != "" || item.SecretBinding != nil {
			return
		}
		policy := policies[item.Key]
		if (policy == types.VMCPConfigurationPolicyUserAllowed) != userInputs {
			return
		}
		key := ConfigurationKey(component.ID, item.Key)
		if (policy != types.VMCPConfigurationPolicyFixed && policy != types.VMCPConfigurationPolicyUserAllowed) || values[key] == "" {
			missing = append(missing, key)
		}
	}
	for _, config := range component.CatalogEntry.Manifest.Config {
		check(config)
	}
	slices.Sort(missing)
	return slices.Compact(missing)
}

// ConfigurationCheckHash identifies the user configuration status of an instance.
// Components must already be limited to those enabled for the instance user.
func ConfigurationCheckHash(enabledComponents []types.VMCPComponent, configurationSyncHash string) string {
	return utils.Digest([]any{enabledComponents, configurationSyncHash})
}
