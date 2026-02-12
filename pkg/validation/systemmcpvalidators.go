package validation

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
)

// ValidateSystemMCPServerManifest validates a SystemMCPServerManifest
func ValidateSystemMCPServerManifest(manifest types.SystemMCPServerManifest) error {
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
	default:
		return types.RuntimeValidationError{
			Runtime: manifest.Runtime,
			Field:   "runtime",
			Message: fmt.Sprintf("SystemMCPServers only support containerized runtime, got: %s", manifest.Runtime),
		}
	}
}
