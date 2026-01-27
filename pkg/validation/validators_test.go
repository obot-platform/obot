package validation

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestUVXValidator_ValidateConfig(t *testing.T) {
	validator := UVXValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "valid uvx config",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "mcp-server-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid uvx config with args",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "mcp-server-package",
					Args:    []string{"--verbose", "--config=file.json"},
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "test"},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "runtime",
				Message: "expected UVX runtime",
			},
		},
		{
			name: "missing uvx config",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "uvxConfig",
				Message: "UVX configuration is required",
			},
		},
		{
			name: "empty package name",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
		{
			name: "whitespace-only package name",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "   \t\n  ",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
		{
			name: "empty arg in args list",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "valid-package",
					Args:    []string{"--valid", "", "--another"},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "args[1]",
				Message: "argument cannot be empty",
			},
		},
		{
			name: "whitespace-only arg",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "valid-package",
					Args:    []string{"--valid", "  \t  "},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "args[1]",
				Message: "argument cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestUVXValidator_ValidateCatalogConfig(t *testing.T) {
	validator := UVXValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerCatalogEntryManifest
		expectedError error
	}{
		{
			name: "valid uvx catalog config",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "catalog-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "test"},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "runtime",
				Message: "expected UVX runtime",
			},
		},
		{
			name: "missing config in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "uvxConfig",
				Message: "UVX configuration is required",
			},
		},
		{
			name: "empty package in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCatalogConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestNPXValidator_ValidateConfig(t *testing.T) {
	validator := NPXValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "valid npx config",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@modelcontextprotocol/server-example",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid npx config with args",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@org/mcp-server",
					Args:    []string{"--port=3000", "--debug"},
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeUVX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "test"},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "runtime",
				Message: "expected NPX runtime",
			},
		},
		{
			name: "missing npx config",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "npxConfig",
				Message: "NPX configuration is required",
			},
		},
		{
			name: "empty package name",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
		{
			name: "whitespace-only package name",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "\t\n\r  ",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
		{
			name: "empty arg in args list",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "valid-package",
					Args:    []string{"--flag", ""},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "args[1]",
				Message: "argument cannot be empty",
			},
		},
		{
			name: "whitespace-only arg",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "valid-package",
					Args:    []string{"\t"},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "args[0]",
				Message: "argument cannot be empty",
			},
		},
		{
			name: "multiple empty args at different positions",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "valid-package",
					Args:    []string{"--valid", "value", "  ", "--another"},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "args[2]",
				Message: "argument cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestNPXValidator_ValidateCatalogConfig(t *testing.T) {
	validator := NPXValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerCatalogEntryManifest
		expectedError error
	}{
		{
			name: "valid npx catalog config",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "catalog-npx-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeContainerized,
				NPXConfig: &types.NPXRuntimeConfig{Package: "test"},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "runtime",
				Message: "expected NPX runtime",
			},
		},
		{
			name: "missing config in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "npxConfig",
				Message: "NPX configuration is required",
			},
		},
		{
			name: "empty package in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCatalogConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestContainerizedValidator_ValidateConfig(t *testing.T) {
	validator := ContainerizedValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "valid containerized config",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "mcp-server:latest",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid with minimum port",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:v1",
					Port:  1,
					Path:  "/",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid with maximum port",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:v1",
					Port:  65535,
					Path:  "/api/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid with args",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "mcp:latest",
					Port:  3000,
					Path:  "/mcp",
					Args:  []string{"--verbose", "--log-level=debug"},
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "/",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "runtime",
				Message: "expected containerized runtime",
			},
		},
		{
			name: "missing containerized config",
			manifest: types.MCPServerManifest{
				Runtime:             types.RuntimeContainerized,
				ContainerizedConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "containerizedConfig",
				Message: "containerized configuration is required",
			},
		},
		{
			name: "empty image",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "image",
				Message: "image field cannot be empty",
			},
		},
		{
			name: "whitespace-only image",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "  \t  ",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "image",
				Message: "image field cannot be empty",
			},
		},
		{
			name: "port zero",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  0,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "port",
				Message: "port must be between 1 and 65535",
			},
		},
		{
			name: "negative port",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  -1,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "port",
				Message: "port must be between 1 and 65535",
			},
		},
		{
			name: "port too large",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  65536,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "port",
				Message: "port must be between 1 and 65535",
			},
		},
		{
			name: "empty path",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "path",
				Message: "path field cannot be empty",
			},
		},
		{
			name: "whitespace-only path",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "   ",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "path",
				Message: "path field cannot be empty",
			},
		},
		{
			name: "empty arg in args list",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "/mcp",
					Args:  []string{"--flag", "", "--another"},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "args[1]",
				Message: "argument cannot be empty",
			},
		},
		{
			name: "whitespace-only arg",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "/mcp",
					Args:  []string{"--flag", "  \n\t  "},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "args[1]",
				Message: "argument cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestContainerizedValidator_ValidateCatalogConfig(t *testing.T) {
	validator := ContainerizedValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerCatalogEntryManifest
		expectedError error
	}{
		{
			name: "valid containerized catalog config",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "catalog-image:v1",
					Port:  9000,
					Path:  "/api/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  8080,
					Path:  "/",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "runtime",
				Message: "expected containerized runtime",
			},
		},
		{
			name: "missing config in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:             types.RuntimeContainerized,
				ContainerizedConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "containerizedConfig",
				Message: "containerized configuration is required",
			},
		},
		{
			name: "invalid port in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "test:latest",
					Port:  100000,
					Path:  "/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeContainerized,
				Field:   "port",
				Message: "port must be between 1 and 65535",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCatalogConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestRemoteValidator_ValidateConfig(t *testing.T) {
	validator := RemoteValidator{}

	tests := []struct {
		name               string
		manifest           types.MCPServerManifest
		expectedError      error
		expectedErrContains string
	}{
		{
			name: "valid remote config with https",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid remote config with http",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "http://localhost:3000/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid with headers",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token"},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "valid template without URL",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:        "",
					IsTemplate: true,
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "runtime",
				Message: "expected remote runtime",
			},
		},
		{
			name: "missing remote config",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "remoteConfig",
				Message: "remote configuration is required",
			},
		},
		{
			name: "empty URL when not template",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:        "",
					IsTemplate: false,
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "url",
				Message: "URL field cannot be empty",
			},
		},
		{
			name: "whitespace-only URL when not template",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "   ",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "url",
				Message: "URL field cannot be empty",
			},
		},
		{
			name: "invalid URL format",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "not a valid url: with spaces",
				},
			},
			expectedErrContains: "invalid URL format",
		},
		{
			name: "invalid URL scheme - ftp",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "ftp://example.com/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "url",
				Message: "URL scheme must be either https or http",
			},
		},
		{
			name: "invalid URL scheme - ws",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "ws://example.com/mcp",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "url",
				Message: "URL scheme must be either https or http",
			},
		},
		{
			name: "empty header key",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "", Value: "some-value"},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "header[0].key",
				Message: "header key cannot be empty",
			},
		},
		{
			name: "whitespace-only header key",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "  \t  ", Value: "value"},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "header[0].key",
				Message: "header key cannot be empty",
			},
		},
		{
			name: "static header marked as sensitive",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "API-Key", Value: "secret123", Sensitive: true},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "header[0]",
				Message: "static header value cannot be marked as sensitive",
			},
		},
		{
			name: "user-configurable header can be sensitive",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "API-Key", Value: "", Sensitive: true, Required: true},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)
			if tt.expectedError == nil && tt.expectedErrContains == "" {
				require.NoError(t, err)
			} else if tt.expectedErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedErrContains)
			} else {
				require.Equal(t, tt.expectedError, err)
			}
		})
	}
}

func TestValidateServerManifest(t *testing.T) {
	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "valid uvx manifest",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "test-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid npx manifest",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@scope/package",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid containerized manifest",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "mcp:latest",
					Port:  8080,
					Path:  "/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid remote manifest",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid composite manifest",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "unsupported runtime",
			manifest: types.MCPServerManifest{
				Runtime: "unsupported-runtime",
			},
			expectedError: types.RuntimeValidationError{
				Runtime: "unsupported-runtime",
				Field:   "runtime",
				Message: "unsupported runtime",
			},
		},
		{
			name: "empty runtime",
			manifest: types.MCPServerManifest{
				Runtime: "",
			},
			expectedError: types.RuntimeValidationError{
				Runtime: "",
				Field:   "runtime",
				Message: "unsupported runtime",
			},
		},
		{
			name: "invalid uvx config propagates error",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeUVX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerManifest(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestValidateCatalogEntryManifest(t *testing.T) {
	tests := []struct {
		name          string
		manifest      types.MCPServerCatalogEntryManifest
		expectedError error
	}{
		{
			name: "valid uvx catalog manifest",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{
					Package: "catalog-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid npx catalog manifest",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "@org/catalog-package",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid containerized catalog manifest",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "catalog:v2",
					Port:  3000,
					Path:  "/api",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid remote catalog manifest",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://catalog.example.com/mcp",
				},
			},
			expectedError: nil,
		},
		{
			name: "valid composite catalog manifest",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "unsupported runtime in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: "unknown-runtime",
			},
			expectedError: types.RuntimeValidationError{
				Runtime: "unknown-runtime",
				Field:   "runtime",
				Message: "unsupported runtime",
			},
		},
		{
			name: "invalid npx config propagates error",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "   ",
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "package",
				Message: "package field cannot be empty",
			},
		},
		{
			name: "invalid remote catalog config propagates error",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "remoteConfig",
				Message: "either fixedURL, hostname, or urlTemplate must be provided",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCatalogEntryManifest(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}

func TestCompositeValidator_ValidateConfig(t *testing.T) {
	validator := CompositeValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "valid composite config",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "wrong runtime",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{CatalogEntryID: "entry-1"},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeNPX,
				Field:   "runtime",
				Message: "expected composite runtime",
			},
		},
		{
			name: "missing composite config",
			manifest: types.MCPServerManifest{
				Runtime:         types.RuntimeComposite,
				CompositeConfig: nil,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig",
				Message: "composite configuration is required",
			},
		},
		{
			name: "no component servers",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers",
				Message: "must contain at least one component server",
			},
		},
		{
			name: "component with both IDs set",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							MCPServerID:    "server-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0]",
				Message: "must have one of catalogEntryID or mcpServerID set",
			},
		},
		{
			name: "component with neither ID set",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0]",
				Message: "must have one of catalogEntryID or mcpServerID set",
			},
		},
		{
			name: "nested composite not allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeComposite,
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].manifest.runtime",
				Message: "runtime cannot be composite",
			},
		},
		{
			name: "duplicate component servers",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[1]",
				Message: "duplicate component server: entry-1",
			},
		},
		{
			name: "valid with MCPServerID",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							MCPServerID: "server-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "mixed CatalogEntryID and MCPServerID",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
						{
							MCPServerID: "server-2",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)
			require.Equal(t, tt.expectedError, err)
		})
	}
}
