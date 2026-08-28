package vmcp

import (
	"github.com/obot-platform/obot/apiclient/types"
)

// IsMultiUser permits a shared runtime only when user inputs are headers.
func IsMultiUser(manifest types.VMCPManifest) bool {
	if manifest.ForceSingleUser {
		return false
	}
	for _, component := range manifest.Components {
		for _, policy := range component.Configuration {
			if policy.Policy != types.VMCPConfigurationPolicyUserAllowed {
				continue
			}

			var isHeader bool
			for _, config := range component.CatalogEntry.Manifest.Config {
				if config.Key == policy.Key {
					isHeader = config.Usage == types.Header
					break
				}
			}
			// Unknown inputs must not share a runtime.
			if !isHeader {
				return false
			}
		}
	}
	return true
}
