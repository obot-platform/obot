package vmcp

import "github.com/obot-platform/obot/apiclient/types"

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
			isHeader := false
			if remote := component.CatalogEntry.Manifest.RemoteConfig; remote != nil {
				for _, header := range remote.Headers {
					isHeader = isHeader || header.Key == policy.Key
				}
			}
			// Ambiguous or unknown inputs must not share a runtime.
			for _, env := range component.CatalogEntry.Manifest.Env {
				if env.Key == policy.Key {
					isHeader = false
				}
			}
			if !isHeader {
				return false
			}
		}
	}
	return true
}
