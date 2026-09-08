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
	}
	return slices.DeleteFunc(components, func(component types.VMCPComponent) bool {
		return slices.Contains(instance.Spec.LegacyDisabledComponents, component.ID)
	})
}
