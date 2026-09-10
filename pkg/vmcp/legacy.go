package vmcp

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

// ComponentsForInstance retains a migrated connection's snapshots until the
// administrator explicitly upgrades the corresponding vMCP component.
func ComponentsForInstance(vmcp v1.VMCP, instance v1.VMCPInstance) []types.VMCPComponent {
	components := vmcp.DeepCopy().Spec.Manifest.Components
	for i, component := range components {
		for _, legacy := range instance.Spec.LegacyComponents {
			if legacy.ID == component.ID && legacy.SourceDigest == utils.Digest([]any{component, vmcp.Spec.ComponentStaticConfigurationHashes[component.ID]}) {
				components[i] = *legacy.DeepCopy()
				break
			}
		}
		// Hostname-constrained catalog entries require a URL for each connection.
		// Carry it through the same credential and consent flow as other user inputs.
		remote := components[i].CatalogEntry.Manifest.RemoteConfig
		if remote != nil && remote.FixedURL == "" && remote.Hostname != "" {
			if !slices.ContainsFunc(components[i].CatalogEntry.Manifest.Config, func(field types.MCPConfig) bool { return field.Key == "__url" }) {
				components[i].CatalogEntry.Manifest.Config = append(components[i].CatalogEntry.Manifest.Config, types.MCPConfig{
					Key:         "__url",
					Name:        "Server URL",
					Description: "URL must have hostname " + remote.Hostname,
					Usage:       types.Interpolated,
					Required:    true,
				})
			}
			if !slices.ContainsFunc(components[i].Configuration, func(policy types.VMCPConfigurationPolicy) bool { return policy.Key == "__url" }) {
				components[i].Configuration = append(components[i].Configuration, types.VMCPConfigurationPolicy{
					Key:    "__url",
					Policy: types.VMCPConfigurationPolicyUserAllowed,
				})
			}
		}
	}
	return slices.DeleteFunc(components, func(component types.VMCPComponent) bool {
		return slices.Contains(instance.Spec.LegacyDisabledComponents, component.ID)
	})
}
