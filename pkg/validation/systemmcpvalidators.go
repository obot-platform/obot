package validation

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
)

// ValidateSystemMCPServerManifest validates a SystemMCPServerManifest
func ValidateSystemMCPServerManifest(manifest types.SystemMCPServerManifest) error {
	// Validate SystemMCPServerType
	switch manifest.SystemMCPServerType {
	case types.SystemMCPServerTypeHook:
		// Valid type
	default:
		return types.RuntimeValidationError{
			Field:   "systemMCPServerType",
			Message: fmt.Sprintf("invalid SystemMCPServerType: %s (only 'hook' is supported)", manifest.SystemMCPServerType),
		}
	}

	// Validate runtime is supported
	switch manifest.Runtime {
	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig == nil {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "containerizedConfig",
				Message: "containerized configuration is required for containerized runtime",
			}
		}
		// Reuse existing containerized validator
		validator := ContainerizedValidator{}
		return validator.validateContainerizedConfig(*manifest.ContainerizedConfig)
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil {
			return types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "remoteConfig",
				Message: "remote configuration is required for remote runtime",
			}
		}
		// Reuse existing remote validator
		validator := RemoteValidator{}
		return validator.validateRemoteConfig(*manifest.RemoteConfig)
	default:
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: fmt.Sprintf("SystemMCPServers only support containerized and remote runtimes, got: %s", manifest.Runtime),
		}
	}
}
