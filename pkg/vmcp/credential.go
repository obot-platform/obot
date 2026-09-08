package vmcp

import (
	"fmt"
	"strings"
	"uuid"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

const (
	configurationCredentialName = "configuration"
	configurationKeyPrefix      = "vmcp.v1."
)

// SetStaticConfigurationHashes records both the deployment-wide synchronization
// hash and the component hashes used to retain unrelated migrated overrides.
func SetStaticConfigurationHashes(vmcp *v1.VMCP, configuration map[string]string) {
	vmcp.Spec.StaticConfigurationHash = utils.Digest(configuration)
	vmcp.Spec.ComponentStaticConfigurationHashes = make(map[string]string, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		values := map[string]string{}
		for _, policy := range component.Configuration {
			if policy.Policy == types.VMCPConfigurationPolicyFixed {
				if value, ok := configuration[ConfigurationKey(component.ID, policy.Key)]; ok {
					values[policy.Key] = value
				}
			}
		}
		vmcp.Spec.ComponentStaticConfigurationHashes[component.ID] = utils.Digest(values)
	}
}

// ConfigurationCredentialName is the name used for VMCP configuration credentials.
func ConfigurationCredentialName() string {
	return configurationCredentialName
}

// StaticConfigurationCredentialContext scopes fixed configuration to a VMCP.
func StaticConfigurationCredentialContext(vmcpID string) string {
	return "vmcp/" + vmcpID
}

// InstanceConfigurationCredentialContext scopes user configuration to a VMCP instance.
func InstanceConfigurationCredentialContext(instanceID string) string {
	return "vmcp-instance/" + instanceID
}

// ConfigurationKey qualifies a configuration key with its VMCP component ID.
// Component IDs cannot contain periods, so configuration keys may contain them
// without making the encoded key ambiguous.
func ConfigurationKey(componentID, key string) string {
	return configurationKeyPrefix + componentID + "." + key
}

// ParseConfigurationKey parses a key produced by ConfigurationKey.
func ParseConfigurationKey(key string) (componentID, configurationKey string, ok bool) {
	remainder, ok := strings.CutPrefix(key, configurationKeyPrefix)
	if !ok {
		return "", "", false
	}

	componentID, configurationKey, ok = strings.Cut(remainder, ".")
	return componentID, configurationKey, ok && componentID != "" && configurationKey != ""
}

// InitializeComponentIDs assigns new IDs to every component during VMCP creation.
// Clients cannot choose IDs because they form part of the credential namespace.
func InitializeComponentIDs(manifest *types.VMCPManifest) error {
	for index := range manifest.Components {
		if manifest.Components[index].ID != "" {
			return fmt.Errorf("component %q ID is server-assigned and must be omitted", manifest.Components[index].Name)
		}
		manifest.Components[index].ID = uuid.New().String()
	}
	return nil
}

// ReconcileComponentIDs preserves IDs during update and assigns IDs to newly
// added components. A supplied ID must already belong to the VMCP.
func ReconcileComponentIDs(existing types.VMCPManifest, desired *types.VMCPManifest) error {
	existingByID := make(map[string]struct{}, len(existing.Components))
	existingByName := make(map[string]string, len(existing.Components))
	for _, component := range existing.Components {
		if component.ID != "" {
			existingByID[component.ID] = struct{}{}
			existingByName[component.Name] = component.ID
		}
	}

	for index := range desired.Components {
		component := &desired.Components[index]
		if component.ID == "" {
			if existingID := existingByName[component.Name]; existingID != "" {
				component.ID = existingID
			} else {
				component.ID = uuid.New().String()
			}
			continue
		}
		if _, ok := existingByID[component.ID]; !ok {
			return fmt.Errorf("component %q has unknown ID %q", component.Name, component.ID)
		}
	}
	return nil
}

// ExtractStaticConfiguration removes fixed values from a manifest and returns
// them in the flat representation used by the credential store. Empty values
// are omitted because they represent missing configuration.
func ExtractStaticConfiguration(manifest *types.VMCPManifest) map[string]string {
	configuration := map[string]string{}
	for componentIndex := range manifest.Components {
		component := &manifest.Components[componentIndex]
		for policyIndex := range component.Configuration {
			policy := &component.Configuration[policyIndex]
			if policy.Policy == types.VMCPConfigurationPolicyFixed && policy.Value != "" {
				configuration[ConfigurationKey(component.ID, policy.Key)] = policy.Value
			}
			policy.Value = ""
		}
	}
	return configuration
}

// ValidateAndEncodeUserConfiguration validates that every supplied value is
// user-allowed by the VMCP and returns the credential-store representation.
func ValidateAndEncodeUserConfiguration(manifest types.VMCPManifest, configuration types.VMCPConfiguration) (map[string]string, error) {
	components := make(map[string]map[string]types.VMCPConfigurationPolicyType, len(manifest.Components))
	for _, component := range manifest.Components {
		policies := make(map[string]types.VMCPConfigurationPolicyType, len(component.Configuration))
		for _, policy := range component.Configuration {
			policies[policy.Key] = policy.Policy
		}
		components[component.ID] = policies
	}

	result := map[string]string{}
	for componentID, values := range configuration.Components {
		policies, ok := components[componentID]
		if !ok {
			return nil, fmt.Errorf("unknown component ID %q", componentID)
		}
		for key, value := range values {
			if policies[key] != types.VMCPConfigurationPolicyUserAllowed {
				return nil, fmt.Errorf("configuration %q for component %q is not user-allowed", key, componentID)
			}
			if value = strings.TrimSpace(value); value != "" {
				result[ConfigurationKey(componentID, key)] = value
			}
		}
	}
	return result, nil
}
