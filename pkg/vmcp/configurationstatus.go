package vmcp

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
		if !item.Required || item.Static || item.SecretBinding != nil {
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
	for _, config := range ComponentConfig(component) {
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

// ServerConfigurationSynced reports whether a component server's configuration
// credential reflects the vMCP's fixed values, the instance user's values, and
// the component snapshot. Shared servers have no instance, so pass an empty one.
func ServerConfigurationSynced(server v1.MCPServer, vmcp v1.VMCP, instance v1.VMCPInstance) bool {
	return server.Status.VMCPStaticConfigurationHash == vmcp.Spec.StaticConfigurationHash &&
		server.Status.VMCPUserConfigurationHash == instance.Status.UserConfigurationHash &&
		server.Status.VMCPSnapshotHash == server.Annotations[v1.VMCPSnapshotDigestAnnotation]
}

// ConnectionConfiguration returns the user-supplied configuration a shared
// component connection carries. It is nil when there is none, matching what
// storage returns for an empty Config.
func ConnectionConfiguration(component types.VMCPComponent) []types.MCPConfig {
	config := slices.DeleteFunc(ComponentConfig(component), func(c types.MCPConfig) bool { return !c.UserAllowed })
	if len(config) == 0 {
		return nil
	}
	return config
}

// ConnectionConfigurationHash identifies the configuration a shared component
// connection was last synchronized with.
func ConnectionConfigurationHash(config []types.MCPConfig, instance v1.VMCPInstance) string {
	return utils.Digest([]any{config, instance.Status.UserConfigurationHash})
}

// ConnectionConfigurationSynced reports whether a shared component connection's
// credential reflects the instance user's current configuration.
func ConnectionConfigurationSynced(connection v1.MCPServerInstance, component types.VMCPComponent, instance v1.VMCPInstance) bool {
	config := ConnectionConfiguration(component)
	return connection.Status.VMCPConfigurationHash == ConnectionConfigurationHash(config, instance) &&
		SameConnectionConfiguration(connection.Spec.Config, config)
}

// SameConnectionConfiguration compares configurations as storage keeps them.
// Storage drops empty slices, so a nil and an empty slice, at any depth, are
// the same configuration.
func SameConnectionConfiguration(a, b []types.MCPConfig) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && bytes.Equal(aJSON, bJSON)
}
