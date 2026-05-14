package validation

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/assert"
)

func TestValidateSystemMCPServerManifest(t *testing.T) {
	tests := []struct {
		name                string
		manifest            types.SystemMCPServerManifest
		expectError         bool
		errorField          string
		expectedErrContains string
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
			name: "valid remote runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
			},
			expectError: false,
		},
		{
			name: "valid npx runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@example/server",
				},
			},
			expectError: false,
		},
		{
			name: "valid uvx runtime",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "example-server",
				},
			},
			expectError: false,
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
		{
			name: "invalid - missing remote config",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeRemote,
			},
			expectError: true,
			errorField:  "remoteConfig",
		},
		{
			name: "invalid - missing npx config",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeNPX,
			},
			expectError: true,
			errorField:  "npxConfig",
		},
		{
			name: "invalid - missing uvx config",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeUVX,
			},
			expectError: true,
			errorField:  "uvxConfig",
		},
		{
			name: "invalid - negative startup timeout",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package:               "test-package",
					StartupTimeoutSeconds: -1,
				},
			},
			expectError: true,
			errorField:  "npxConfig.startupTimeoutSeconds",
		},
		{
			name: "invalid - startup timeout above maximum",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image:                 "test-image",
					Port:                  8080,
					Path:                  "/mcp",
					StartupTimeoutSeconds: int(mcp.MaxMCPServerStartupTimeout.Seconds()) + 1,
				},
			},
			expectError: true,
			errorField:  "containerizedConfig.startupTimeoutSeconds",
		},
		{
			name: "invalid - env secret binding is not allowed",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@example/server",
				},
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Key: "API_KEY",
						SecretBinding: &types.MCPSecretBinding{
							Name: "my-secret",
							Key:  "token",
						},
					},
				}},
			},
			expectError:         true,
			expectedErrContains: "secretBinding is not supported for system MCP servers",
		},
		{
			name: "invalid - remote header secret binding is not allowed",
			manifest: types.SystemMCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{{
						Key: "Authorization",
						SecretBinding: &types.MCPSecretBinding{
							Name: "my-secret",
							Key:  "token",
						},
					}},
				},
			},
			expectError:         true,
			expectedErrContains: "secretBinding is not supported for system MCP servers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSystemMCPServerManifest(tt.manifest)
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrContains != "" {
					assert.Contains(t, err.Error(), tt.expectedErrContains)
				}
				if validationErr, ok := err.(types.RuntimeValidationError); ok {
					assert.Equal(t, tt.errorField, validationErr.Field)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
