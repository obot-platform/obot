package validation

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
)

func TestValidateSystemMCPServerManifest(t *testing.T) {
	tests := []struct {
		name        string
		manifest    types.SystemMCPServerManifest
		expectError bool
		errorField  string
	}{
		{
			name: "valid containerized hook",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectError: false,
		},
		{
			name: "invalid - remote runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeRemote,
			},
			expectError: true,
			errorField:  "runtime",
		},
		{
			name: "invalid - npx runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeNPX,
			},
			expectError: true,
			errorField:  "runtime",
		},
		{
			name: "invalid - uvx runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeUVX,
			},
			expectError: true,
			errorField:  "runtime",
		},
		{
			name: "invalid - composite runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeComposite,
			},
			expectError: true,
			errorField:  "runtime",
		},
		{
			name: "invalid - missing containerized config",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
			},
			expectError: true,
			errorField:  "containerizedConfig",
		},
		{
			name: "invalid - containerized with no image",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Port: 8080,
					Path: "/mcp",
				},
			},
			expectError: true,
			errorField:  "image",
		},
		{
			name: "invalid - containerized with invalid port",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  0,
					Path:  "/mcp",
				},
			},
			expectError: true,
			errorField:  "port",
		},
		{
			name: "invalid - containerized with no path",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
				},
			},
			expectError: true,
			errorField:  "path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSystemMCPServerManifest(tt.manifest)
			if tt.expectError {
				assert.Error(t, err)
				if validationErr, ok := err.(types.RuntimeValidationError); ok {
					assert.Equal(t, tt.errorField, validationErr.Field)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
