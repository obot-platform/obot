package validation

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
)

func TestValidateServerManifestForCatalog_MultiUserConfig(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "test-server",
		},
	}

	require.NoError(t, ValidateServerManifest(manifest, true))

	manifest.MultiUserConfig = &types.MultiUserConfig{}
	require.Equal(t, types.RuntimeValidationError{
		Runtime: types.RuntimeNPX,
		Field:   "multiUserConfig",
		Message: "multiUserConfig may only be set for multi-user servers",
	}, ValidateServerManifest(manifest, false))
	require.NoError(t, ValidateServerManifest(manifest, true))
}

func TestRemoteValidator_validateRemoteCatalogConfig(t *testing.T) {
	validator := RemoteValidator{}

	tests := []struct {
		name        string
		config      types.RemoteCatalogConfig
		expectError bool
		errorField  string
		errorMsg    string
	}{
		// Valid cases - FixedURL only
		{
			name: "valid fixedURL with https",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://api.example.com/mcp",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with http",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://localhost:3000/mcp",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with port",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://api.example.com:8080/mcp",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with path and query",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://api.example.com/mcp/endpoint?param=value",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with IP address",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://192.168.1.1:8080/mcp",
			},
			expectError: false,
		},

		// Valid cases - Hostname only
		{
			name: "valid hostname simple",
			config: types.RemoteCatalogConfig{
				Hostname: "example.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with subdomain",
			config: types.RemoteCatalogConfig{
				Hostname: "api.example.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with multiple subdomains",
			config: types.RemoteCatalogConfig{
				Hostname: "api.v1.example.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with wildcard",
			config: types.RemoteCatalogConfig{
				Hostname: "*.example.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with wildcard and subdomain",
			config: types.RemoteCatalogConfig{
				Hostname: "*.api.example.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with numbers",
			config: types.RemoteCatalogConfig{
				Hostname: "api1.example2.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with hyphens",
			config: types.RemoteCatalogConfig{
				Hostname: "api-server.example-site.com",
			},
			expectError: false,
		},
		{
			name: "valid hostname with wildcard and hyphens",
			config: types.RemoteCatalogConfig{
				Hostname: "*.api-server.example-site.com",
			},
			expectError: false,
		},

		// Valid cases - with Headers
		{
			name: "valid fixedURL with headers",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://api.example.com/mcp",
				Headers: []types.MCPHeader{
					{Name: "Authorization", Key: "Bearer token"},
					{Name: "Content-Type", Key: "application/json"},
				},
			},
			expectError: false,
		},
		{
			name: "valid hostname with headers",
			config: types.RemoteCatalogConfig{
				Hostname: "*.example.com",
				Headers: []types.MCPHeader{
					{Name: "X-API-Key", Key: "secret"},
				},
			},
			expectError: false,
		},

		// Valid cases - URLTemplate only
		{
			name: "valid urlTemplate with single variable",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/mcp/endpoint",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with multiple variables",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with path and query",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/api/${VERSION}/endpoint?token=${API_TOKEN}&user=${USER_ID}",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with port",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}:${PORT}/mcp",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with complex path",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${REGION}.${SERVICE}.${PROVIDER}.com/${VERSION}/${RESOURCE}/${ID}",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with special characters in variables",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/api/${USER_NAME}/profile",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with underscore in variables",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/api/${USER_ID}/data",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with numbers in variables",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/api/v${VERSION}/endpoint",
			},
			expectError: false,
		},

		// Valid cases - URLTemplate with Headers
		{
			name: "valid urlTemplate with headers",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/mcp",
				Headers: []types.MCPHeader{
					{Name: "Authorization", Key: "Bearer token"},
					{Name: "X-API-Key", Key: "secret"},
				},
			},
			expectError: false,
		},

		// Valid cases - URLTemplate with mixed configurations
		{
			name: "valid urlTemplate with http scheme",
			config: types.RemoteCatalogConfig{
				URLTemplate: "http://${API_HOST}/mcp",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with IP address variable",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${SERVER_IP}:${PORT}/mcp",
			},
			expectError: false,
		},
		{
			name: "valid urlTemplate with subdomain variables",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${ENV}.${SERVICE}.${DOMAIN}.com/mcp",
			},
			expectError: false,
		},

		// Error cases - missing both
		{
			name:        "empty config",
			config:      types.RemoteCatalogConfig{},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "either fixedURL, hostname, or urlTemplate must be provided",
		},
		{
			name: "all fields empty strings",
			config: types.RemoteCatalogConfig{
				FixedURL:    "",
				Hostname:    "",
				URLTemplate: "",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "either fixedURL, hostname, or urlTemplate must be provided",
		},
		{
			name: "all fields whitespace only",
			config: types.RemoteCatalogConfig{
				FixedURL:    "   ",
				Hostname:    "\t\n",
				URLTemplate: "  ",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "either fixedURL, hostname, or urlTemplate must be provided",
		},

		// Error cases - multiple fields provided
		{
			name: "both fixedURL and hostname provided",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://api.example.com/mcp",
				Hostname: "example.com",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "both fixedURL and urlTemplate provided",
			config: types.RemoteCatalogConfig{
				FixedURL:    "https://api.example.com/mcp",
				URLTemplate: "https://${API_HOST}/mcp",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "both hostname and urlTemplate provided",
			config: types.RemoteCatalogConfig{
				Hostname:    "example.com",
				URLTemplate: "https://${API_HOST}/mcp",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "all three fields provided",
			config: types.RemoteCatalogConfig{
				FixedURL:    "https://api.example.com/mcp",
				Hostname:    "example.com",
				URLTemplate: "https://${API_HOST}/mcp",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},

		// Additional test cases for comprehensive coverage
		{
			name: "fixedURL and hostname with whitespace",
			config: types.RemoteCatalogConfig{
				FixedURL: " https://api.example.com/mcp ",
				Hostname: " example.com ",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "fixedURL and urlTemplate with whitespace",
			config: types.RemoteCatalogConfig{
				FixedURL:    " https://api.example.com/mcp ",
				URLTemplate: " https://${API_HOST}/mcp ",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "hostname and urlTemplate with whitespace",
			config: types.RemoteCatalogConfig{
				Hostname:    " example.com ",
				URLTemplate: " https://${API_HOST}/mcp ",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},
		{
			name: "all three fields with whitespace",
			config: types.RemoteCatalogConfig{
				FixedURL:    " https://api.example.com/mcp ",
				Hostname:    " example.com ",
				URLTemplate: " https://${API_HOST}/mcp ",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "cannot specify multiple URL configuration methods",
		},

		// Error cases - invalid FixedURL
		{
			name: "invalid fixedURL - malformed",
			config: types.RemoteCatalogConfig{
				FixedURL: "not-a-valid-url",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "URL scheme must be either https or http",
		},
		{
			name: "invalid fixedURL - missing scheme",
			config: types.RemoteCatalogConfig{
				FixedURL: "example.com/path",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "URL scheme must be either https or http",
		},

		// Error cases - invalid Hostname
		{
			name: "invalid hostname - contains underscore",
			config: types.RemoteCatalogConfig{
				Hostname: "api_server.example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - contains spaces",
			config: types.RemoteCatalogConfig{
				Hostname: "api server.example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - contains special characters",
			config: types.RemoteCatalogConfig{
				Hostname: "api@example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - starts with dot",
			config: types.RemoteCatalogConfig{
				Hostname: ".example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - ends with dot",
			config: types.RemoteCatalogConfig{
				Hostname: "example.com.",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - double dots",
			config: types.RemoteCatalogConfig{
				Hostname: "api..example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - wildcard in wrong position",
			config: types.RemoteCatalogConfig{
				Hostname: "api.*.example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - multiple wildcards",
			config: types.RemoteCatalogConfig{
				Hostname: "*.*.example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - wildcard without dot",
			config: types.RemoteCatalogConfig{
				Hostname: "*example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - contains port",
			config: types.RemoteCatalogConfig{
				Hostname: "example.com:8080",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - contains path",
			config: types.RemoteCatalogConfig{
				Hostname: "example.com/path",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "invalid hostname - contains protocol",
			config: types.RemoteCatalogConfig{
				Hostname: "https://example.com",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},

		// Edge cases
		{
			name: "fixedURL with whitespace",
			config: types.RemoteCatalogConfig{
				FixedURL: "  https://api.example.com/mcp  ",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "invalid URL format",
		},
		{
			name: "hostname with whitespace gets trimmed",
			config: types.RemoteCatalogConfig{
				Hostname: "  example.com  ",
			},
			expectError: true,
			errorField:  "hostname",
			errorMsg:    "hostname should only contain alphanumeric and hyphens",
		},
		{
			name: "single character hostname",
			config: types.RemoteCatalogConfig{
				Hostname: "a",
			},
			expectError: false,
		},
		{
			name: "single character with wildcard",
			config: types.RemoteCatalogConfig{
				Hostname: "*.a",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateRemoteCatalogConfig(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}

				// Check if it's a RuntimeValidationError
				validationErr, ok := err.(types.RuntimeValidationError)
				if !ok {
					t.Errorf("expected RuntimeValidationError, got %T", err)
					return
				}

				// Check runtime
				if validationErr.Runtime != types.RuntimeRemote {
					t.Errorf("expected runtime %s, got %s", types.RuntimeRemote, validationErr.Runtime)
				}

				// Check field
				if validationErr.Field != tt.errorField {
					t.Errorf("expected field %s, got %s", tt.errorField, validationErr.Field)
				}

				// Check message contains expected text
				if tt.errorMsg != "" && !strings.Contains(validationErr.Message, tt.errorMsg) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.errorMsg, validationErr.Message)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoteValidator_validateRemoteCatalogConfig_HostnameRegexEdgeCases(t *testing.T) {
	validator := RemoteValidator{}

	// Additional regex-specific test cases
	regexTests := []struct {
		name        string
		hostname    string
		expectError bool
	}{
		// Valid cases that might be edge cases for regex
		{"valid single letter domain", "a.b", false},
		{"valid numbers only", "123.456", false},
		{"valid mixed alphanumeric", "a1b2.c3d4", false},
		{"valid long hostname", "very-long-subdomain-name.very-long-domain-name.com", false},
		{"valid wildcard with single char", "*.a", false},
		{"valid deep subdomain", "a.b.c.d.e.f.g.h", false},

		// Invalid cases for regex
		{"empty string", "", true},
		{"just wildcard", "*", true},
		{"just dot", ".", true},
		{"starts with dot", ".example.com", true},
		{"ends with dot", "example.com.", true},
		{"consecutive dots", "example..com", true},
		{"wildcard not at start", "sub.*.example.com", true},
		{"multiple wildcards", "*.*.example.com", true},
		{"wildcard without dot", "*example.com", true},
		{"contains slash", "example.com/path", true},
		{"contains colon", "example.com:8080", true},
		{"contains question mark", "example.com?query", true},
		{"contains hash", "example.com#fragment", true},
		{"contains at sign", "user@example.com", true},
		{"contains space", "example .com", true},
		{"contains tab", "example\t.com", true},
		{"contains newline", "example\n.com", true},
		{"unicode characters", "exämple.com", true},
		{"chinese characters", "例え.com", true},
	}

	for _, tt := range regexTests {
		t.Run(tt.name, func(t *testing.T) {
			config := types.RemoteCatalogConfig{
				Hostname: tt.hostname,
			}

			err := validator.validateRemoteCatalogConfig(config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error for hostname '%s' but got none", tt.hostname)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for hostname '%s': %v", tt.hostname, err)
				}
			}
		})
	}
}

func TestRemoteValidator_ValidateConfig_HeaderValidation(t *testing.T) {
	validator := RemoteValidator{}

	tests := []struct {
		name        string
		manifest    types.MCPServerManifest
		expectError bool
		errorField  string
		errorMsg    string
	}{
		{
			name: "valid headers",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token"},
						{Key: "Content-Type", Value: "application/json"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty header key should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "", Value: "some-value"},
					},
				},
			},
			expectError: true,
			errorField:  "header[0].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "whitespace-only header key should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "   ", Value: "some-value"},
					},
				},
			},
			expectError: true,
			errorField:  "header[0].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "static header marked as sensitive should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token", Sensitive: true},
					},
				},
			},
			expectError: true,
			errorField:  "header[0]",
			errorMsg:    "static header value cannot be marked as sensitive",
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
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(tt.manifest)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}

				var runtimeErr types.RuntimeValidationError
				if errors.As(err, &runtimeErr) {
					if runtimeErr.Field != tt.errorField {
						t.Errorf("expected error field %q, got %q", tt.errorField, runtimeErr.Field)
					}
					if !strings.Contains(runtimeErr.Message, tt.errorMsg) {
						t.Errorf("expected error message to contain %q, got %q", tt.errorMsg, runtimeErr.Message)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoteValidator_ValidateCatalogConfig_HeaderValidation(t *testing.T) {
	validator := RemoteValidator{}

	tests := []struct {
		name        string
		manifest    types.MCPServerCatalogEntryManifest
		expectError bool
		errorField  string
		errorMsg    string
	}{
		{
			name: "valid headers",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token"},
						{Key: "Content-Type", Value: "application/json"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty header key should fail",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "", Value: "some-value"},
					},
				},
			},
			expectError: true,
			errorField:  "header[0].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "multiple headers with one empty key should fail",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Valid-Header", Value: "valid-value"},
						{Key: "", Value: "invalid-value"},
					},
				},
			},
			expectError: true,
			errorField:  "header[1].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "static header marked as sensitive should fail",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
					Headers: []types.MCPHeader{
						{Key: "Authorization", Value: "Bearer token", Sensitive: true},
					},
				},
			},
			expectError: true,
			errorField:  "header[0]",
			errorMsg:    "static header value cannot be marked as sensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateCatalogConfig(tt.manifest)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}

				var runtimeErr types.RuntimeValidationError
				if errors.As(err, &runtimeErr) {
					if runtimeErr.Field != tt.errorField {
						t.Errorf("expected error field %q, got %q", tt.errorField, runtimeErr.Field)
					}
					if !strings.Contains(runtimeErr.Message, tt.errorMsg) {
						t.Errorf("expected error message to contain %q, got %q", tt.errorMsg, runtimeErr.Message)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateEgressDomains(t *testing.T) {
	tests := []struct {
		name          string
		runtime       types.Runtime
		domains       []string
		denyAllEgress *bool
		expectError   bool
		errorMsg      string
	}{
		{
			name:    "accept exact domain",
			runtime: types.RuntimeNPX,
			domains: []string{"api.example.com"},
		},
		{
			name:    "accept wildcard domain",
			runtime: types.RuntimeUVX,
			domains: []string{"*.example.com"},
		},
		{
			name:        "reject protocol",
			runtime:     types.RuntimeNPX,
			domains:     []string{"https://example.com"},
			expectError: true,
			errorMsg:    "must not include a protocol",
		},
		{
			name:        "reject path",
			runtime:     types.RuntimeUVX,
			domains:     []string{"example.com/path"},
			expectError: true,
			errorMsg:    "must not include a path or port",
		},
		{
			name:        "reject port",
			runtime:     types.RuntimeContainerized,
			domains:     []string{"example.com:443"},
			expectError: true,
			errorMsg:    "must not include a path or port",
		},
		{
			name:        "reject mid label wildcard",
			runtime:     types.RuntimeNPX,
			domains:     []string{"foo.*.example.com"},
			expectError: true,
			errorMsg:    "must be a valid hostname",
		},
		{
			name:        "reject empty domain",
			runtime:     types.RuntimeUVX,
			domains:     []string{" "},
			expectError: true,
			errorMsg:    "cannot be empty",
		},
		{
			name:        "reject wildcard all",
			runtime:     types.RuntimeContainerized,
			domains:     []string{"*"},
			expectError: true,
			errorMsg:    "must be a valid hostname or leading wildcard hostname",
		},
		{
			name:        "reject IP address",
			runtime:     types.RuntimeNPX,
			domains:     []string{"169.254.169.254"},
			expectError: true,
			errorMsg:    "must not be an IP address",
		},
		{
			name:        "reject single-label host",
			runtime:     types.RuntimeUVX,
			domains:     []string{"metadata"},
			expectError: true,
			errorMsg:    "at least two DNS labels",
		},
		{
			name:        "reject broad wildcard",
			runtime:     types.RuntimeContainerized,
			domains:     []string{"*.com"},
			expectError: true,
			errorMsg:    "at least two DNS labels",
		},
		{
			name:        "reject cluster internal domain",
			runtime:     types.RuntimeNPX,
			domains:     []string{"*.svc.cluster.local"},
			expectError: true,
			errorMsg:    "is not allowed",
		},
		{
			name:        "reject reverse DNS domain",
			runtime:     types.RuntimeNPX,
			domains:     []string{"*.254.169.in-addr.arpa"},
			expectError: true,
			errorMsg:    "is not allowed",
		},
		{
			name:          "reject domains when deny all egress enabled",
			runtime:       types.RuntimeNPX,
			domains:       []string{"example.com"},
			denyAllEgress: new(true),
			expectError:   true,
			errorMsg:      "denyAllEgress cannot be true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEgressDomains(tt.runtime, tt.domains, tt.denyAllEgress)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMsg)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestCompositeValidator_ValidateCatalogConfig(t *testing.T) {
	validator := CompositeValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerCatalogEntryManifest
		expectedError error
	}{
		{
			name: "non-composite runtime",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeRemote,
				Field:   "runtime",
				Message: "expected composite runtime",
			},
		},
		{
			name: "missing composite config",
			manifest: types.MCPServerCatalogEntryManifest{
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
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers",
				Message: "must contain at least one component server",
			},
		},
		{
			name: "component missing both IDs",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "",
							MCPServerID:    "",
							Manifest: types.MCPServerCatalogEntryManifest{
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
			name: "component with both IDs set",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							MCPServerID:    "server-1",
							Manifest: types.MCPServerCatalogEntryManifest{
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
			name: "nested composite runtime not allowed in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
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
			name: "duplicate component servers detected in catalog",
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
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
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
			name: "valid catalog composite configuration passes",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{
									Name:         "tool-1",
									OverrideName: "tool-1",
									Enabled:      true,
								},
							},
						},
						{
							MCPServerID: "server-2",
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
			name: "remote component with static OAuth not allowed in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteCatalogConfig{
									FixedURL:            "https://example.com/mcp",
									StaticOAuthRequired: true,
								},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0]",
				Message: "remote component with static OAuth cannot be included in a composite server",
			},
		},
		{
			name: "remote component without static OAuth is allowed in catalog",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteCatalogConfig{
									FixedURL:            "https://example.com/mcp",
									StaticOAuthRequired: false,
								},
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "tool prefix with invalid character",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
							ToolPrefix: "gh space",
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolPrefix",
				Message: "toolPrefix must match ^[A-Za-z0-9._/-]*$",
			},
		},
		{
			name: "tool prefix is allowed without tool overrides",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: "gh_",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "duplicate non-empty tool prefix",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
							ToolPrefix: "gh_",
						},
						{
							CatalogEntryID: "entry-2",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
							ToolPrefix: "gh_",
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[1].toolPrefix",
				Message: "duplicate toolPrefix: gh_",
			},
		},
		{
			name: "valid tool prefix with tool overrides",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
							ToolPrefix: "gh_",
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "tool prefix at max length is allowed",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: strings.Repeat("a", maxToolPrefixLength),
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "tool prefix exceeds max length",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: strings.Repeat("a", maxToolPrefixLength+1),
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolPrefix",
				Message: "toolPrefix must be at most 64 characters",
			},
		},
		{
			name: "empty tool prefixes may repeat across components",
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
						{
							CatalogEntryID: "entry-2",
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
			name: "empty original tool name",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "", Enabled: true},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolOverrides[0].name",
				Message: "original tool name is required",
			},
		},
		{
			name: "effective tool name exceeds max length",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: strings.Repeat("a", maxToolPrefixLength),
							ToolOverrides: []types.ToolOverride{
								{Name: strings.Repeat("b", maxToolNameLength-maxToolPrefixLength+1), Enabled: true},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolOverrides[0]",
				Message: fmt.Sprintf(
					"effective tool name must be at most 128 characters: %q",
					strings.Repeat("a", maxToolPrefixLength)+strings.Repeat("b", maxToolNameLength-maxToolPrefixLength+1),
				),
			},
		},
		{
			name: "effective tool name has invalid character",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list things", Enabled: true},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolOverrides[0]",
				Message: "effective tool name must match ^[A-Za-z0-9._/-]*$",
			},
		},
		{
			name: "duplicate effective tool name across components",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
						},
						{
							CatalogEntryID: "entry-2",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolOverrides: []types.ToolOverride{
								{Name: "list", Enabled: true},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[1].toolOverrides[0]",
				Message: "duplicate tool name: list",
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

func TestCompositeValidator_ValidateConfig_StaticOAuth(t *testing.T) {
	validator := CompositeValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "remote component with static OAuth not allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteRuntimeConfig{
									URL:                 "https://example.com/mcp",
									StaticOAuthRequired: true,
								},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0]",
				Message: "remote component with static OAuth cannot be included in a composite server",
			},
		},
		{
			name: "remote component without static OAuth is allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteRuntimeConfig{
									URL:                 "https://example.com/mcp",
									StaticOAuthRequired: false,
								},
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "non-remote component in composite is allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeUVX,
								UVXConfig: &types.UVXRuntimeConfig{
									Package: "mcp-server-test",
								},
							},
						},
					},
				},
			},
			expectedError: nil,
		},
		{
			name: "mixed components with one having static OAuth not allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteRuntimeConfig{
									URL:                 "https://example.com/mcp",
									StaticOAuthRequired: false,
								},
							},
						},
						{
							CatalogEntryID: "entry-2",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteRuntimeConfig{
									URL:                 "https://oauth.example.com/mcp",
									StaticOAuthRequired: true,
								},
							},
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[1]",
				Message: "remote component with static OAuth cannot be included in a composite server",
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

func TestCompositeValidator_ValidateConfig_ToolPrefixLength(t *testing.T) {
	validator := CompositeValidator{}

	tests := []struct {
		name          string
		manifest      types.MCPServerManifest
		expectedError error
	}{
		{
			name: "tool prefix at max length is allowed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: strings.Repeat("a", maxToolPrefixLength),
						},
					},
				},
			},
		},
		{
			name: "tool prefix exceeds max length",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{
							CatalogEntryID: "entry-1",
							Manifest: types.MCPServerManifest{
								Runtime: types.RuntimeRemote,
							},
							ToolPrefix: strings.Repeat("a", maxToolPrefixLength+1),
						},
					},
				},
			},
			expectedError: types.RuntimeValidationError{
				Runtime: types.RuntimeComposite,
				Field:   "compositeConfig.componentServers[0].toolPrefix",
				Message: "toolPrefix must be at most 64 characters",
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

func TestValidateManifestStartupTimeoutNonNegative(t *testing.T) {
	t.Run("server manifest rejects negative startup timeout", func(t *testing.T) {
		err := ValidateServerManifest(types.MCPServerManifest{
			Runtime: types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: -1,
			},
		}, false)

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig.startupTimeoutSeconds",
			Message: "must be greater than or equal to 0",
		}, err)
	})

	t.Run("catalog manifest rejects negative startup timeout", func(t *testing.T) {
		err := ValidateCatalogEntryManifest(types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeUVX,
			UVXConfig: &types.UVXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: -1,
			},
		})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeUVX,
			Field:   "uvxConfig.startupTimeoutSeconds",
			Message: "must be greater than or equal to 0",
		}, err)
	})

	t.Run("server manifest rejects startup timeout above maximum", func(t *testing.T) {
		maxStartupTimeoutSeconds := int(mcp.MaxMCPServerStartupTimeout.Seconds())
		err := ValidateServerManifest(types.MCPServerManifest{
			Runtime: types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image:                 "test-image",
				Port:                  8080,
				Path:                  "/mcp",
				StartupTimeoutSeconds: maxStartupTimeoutSeconds + 1,
			},
		}, false)

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "containerizedConfig.startupTimeoutSeconds",
			Message: fmt.Sprintf("must be less than %d", maxStartupTimeoutSeconds),
		}, err)
	})

	t.Run("catalog manifest rejects startup timeout above maximum", func(t *testing.T) {
		maxStartupTimeoutSeconds := int(mcp.MaxMCPServerStartupTimeout.Seconds())
		err := ValidateCatalogEntryManifest(types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: maxStartupTimeoutSeconds + 1,
			},
		})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig.startupTimeoutSeconds",
			Message: fmt.Sprintf("must be less than %d", maxStartupTimeoutSeconds),
		}, err)
	})
}

func TestValidateSecretBindings(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "datadog-prod", Key: "api-key"}

	tests := []struct {
		name       string
		manifest   types.MCPServerManifest
		gitManaged bool
		backend    string
		wantErr    string // substring; "" = expect no error
	}{
		{
			name: "no bindings is allowed regardless",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "X-Foo", Value: "bar"}},
				},
			},
			gitManaged: false,
			backend:    "docker",
		},
		{
			name: "bound header requires git-managed",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "DD-API-KEY", SecretBinding: binding}},
				},
			},
			gitManaged: false,
			backend:    "kubernetes",
			wantErr:    "git-synced catalog entries",
		},
		{
			name: "bound header accepted for git-managed remote",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "DD-API-KEY", SecretBinding: binding}},
				},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "bound header rejected on non-kubernetes backend",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "DD-API-KEY", SecretBinding: binding}},
				},
			},
			gitManaged: true,
			backend:    "docker",
			wantErr:    "requires the kubernetes MCP runtime backend",
		},
		{
			name: "binding and static value are mutually exclusive",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "DD-API-KEY", Value: "literal", SecretBinding: binding}},
				},
			},
			gitManaged: true,
			backend:    "kubernetes",
			wantErr:    "mutually exclusive",
		},
		{
			name: "binding requires non-empty name/key",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					Headers: []types.MCPHeader{{Key: "DD-API-KEY", SecretBinding: &types.MCPSecretBinding{Name: "datadog-prod"}}},
				},
			},
			gitManaged: true,
			backend:    "kubernetes",
			wantErr:    "requires both name and key",
		},
		{
			name: "bound env under remote runtime is rejected",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				Env:          []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "DD_API_KEY", SecretBinding: binding}}},
				RemoteConfig: &types.RemoteRuntimeConfig{},
			},
			gitManaged: true,
			backend:    "kubernetes",
			wantErr:    "not supported for remote runtime",
		},
		{
			name: "file-backed env with secret binding is accepted",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "DD_API_KEY", SecretBinding: binding}, File: true}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "bound env accepted for git-managed containerized",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "DD_API_KEY", SecretBinding: binding}}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "dynamicFile without file is accepted and ignored",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{
					MCPHeader:   types.MCPHeader{Key: "DD_API_KEY", SecretBinding: binding},
					DynamicFile: true,
				}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "file and dynamicFile together are accepted",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{
					MCPHeader:   types.MCPHeader{Key: "DD_API_KEY"},
					File:        true,
					DynamicFile: true,
				}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindings(tt.manifest, tt.gitManaged, tt.backend)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateSecretBindingsCatalogEntry_URLTemplate(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "my-secret", Key: "token"}

	tests := []struct {
		name     string
		manifest types.MCPServerCatalogEntryManifest
		wantErr  string
	}{
		{
			name: "urlTemplate referencing non-bound env is allowed",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
					Key: "HOST", Required: true,
				}}},
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: "https://${HOST}/mcp",
				},
			},
		},
		{
			name: "urlTemplate referencing secret-bound env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
					Key: "TOKEN", Required: true, SecretBinding: binding,
				}}},
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: "https://example.com/${TOKEN}/mcp",
				},
			},
			wantErr: "remoteConfig.urlTemplate references secret-bound env var",
		},
		{
			name: "no urlTemplate with bound env passes to core check",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
					Key: "TOKEN", Required: true, SecretBinding: binding,
				}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindingsCatalogEntry(tt.manifest, true, "kubernetes")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTemplateReferences_Server(t *testing.T) {
	required := types.MCPEnv{MCPHeader: types.MCPHeader{Key: "TAG", Required: true}}
	optional := types.MCPEnv{MCPHeader: types.MCPHeader{Key: "TAG", Required: false}}

	tests := []struct {
		name     string
		manifest types.MCPServerManifest
		wantErr  string // substring; "" = expect no error
	}{
		{
			name: "no templates is fine",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--flag", "value"}},
			},
		},
		{
			name: "npx templated arg with required env passes",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Env:       []types.MCPEnv{required},
			},
		},
		{
			name: "npx templated arg with optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Env:       []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "uvx templated command with required env passes",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "pkg", Command: "${TAG}"},
				Env:       []types.MCPEnv{required},
			},
		},
		{
			name: "containerized templated arg with optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "img",
					Args:  []string{"--tag=${TAG}"},
				},
				Env: []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote URL template with optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://${TAG}.example.com/mcp",
				},
				Env: []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote header value templated by optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:     "https://example.com/mcp",
					Headers: []types.MCPHeader{{Key: "Authorization", Value: "Bearer ${TAG}"}},
				},
				Env: []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "undeclared template ref is tolerated for server manifests",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				// no env declared — auto-extraction will add Required=true later
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateReferences(tt.manifest)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTemplateReferences_CatalogEntry(t *testing.T) {
	required := types.MCPEnv{MCPHeader: types.MCPHeader{Key: "TAG", Required: true}}
	optional := types.MCPEnv{MCPHeader: types.MCPHeader{Key: "TAG", Required: false}}

	tests := []struct {
		name     string
		manifest types.MCPServerCatalogEntryManifest
		wantErr  string
	}{
		{
			name: "templated arg with required env passes",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Env:       []types.MCPEnv{required},
			},
		},
		{
			name: "templated arg with undeclared env is rejected for catalog entries",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				// no env declared
			},
			wantErr: "undeclared",
		},
		{
			name: "remote FixedURL template with required env passes",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://${TAG}.example.com/mcp",
				},
				Env: []types.MCPEnv{required},
			},
		},
		{
			name: "npx templated arg with optional env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Env:       []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote URLTemplate with optional env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: "https://${TAG}.example.com/mcp",
				},
				Env: []types.MCPEnv{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote header value templated by undeclared env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com/mcp",
					Headers:  []types.MCPHeader{{Key: "Authorization", Value: "Bearer ${TAG}"}},
				},
			},
			wantErr: "undeclared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateReferencesCatalogEntry(tt.manifest)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateCatalogEntryForRoute(t *testing.T) {
	singleUserManifest := types.MCPServerCatalogEntryManifest{ServerUserType: types.ServerUserTypeSingleUser}
	multiUserManifest := types.MCPServerCatalogEntryManifest{ServerUserType: types.ServerUserTypeMultiUser}
	defaultManifest := types.MCPServerCatalogEntryManifest{} // empty = singleUser

	tests := []struct {
		name        string
		manifest    types.MCPServerCatalogEntryManifest
		catalogID   string
		workspaceID string
		expectError bool
	}{
		{
			name:        "single-user entry on single-user route: ok",
			manifest:    singleUserManifest,
			catalogID:   "",
			workspaceID: "",
			expectError: false,
		},
		{
			name:        "empty serverUserType on single-user route: ok",
			manifest:    defaultManifest,
			catalogID:   "",
			workspaceID: "",
			expectError: false,
		},
		{
			name:        "multiUser entry on single-user route: rejected",
			manifest:    multiUserManifest,
			catalogID:   "",
			workspaceID: "",
			expectError: true,
		},
		{
			name:        "single-user entry on catalog route: rejected (deploying catalog entries as multi-user not yet supported)",
			manifest:    singleUserManifest,
			catalogID:   "default",
			workspaceID: "",
			expectError: true,
		},
		{
			name:        "single-user entry on workspace route: rejected (deploying catalog entries as multi-user not yet supported)",
			manifest:    singleUserManifest,
			catalogID:   "",
			workspaceID: "ws-1",
			expectError: true,
		},
		{
			name:        "multiUser entry on catalog route: rejected (deploying catalog entries as multi-user not yet supported)",
			manifest:    multiUserManifest,
			catalogID:   "default",
			workspaceID: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCatalogEntryForRoute(tt.manifest, tt.catalogID, tt.workspaceID)
			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCatalogEntryManifest_ServerUserType(t *testing.T) {
	baseManifest := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "test-server",
		},
	}

	tests := []struct {
		name           string
		serverUserType types.ServerUserType
		expectError    bool
	}{
		{
			name:           "empty serverUserType is valid (defaults to singleUser)",
			serverUserType: "",
			expectError:    false,
		},
		{
			name:           "explicit singleUser is valid",
			serverUserType: types.ServerUserTypeSingleUser,
			expectError:    false,
		},
		{
			name:           "multiUser is rejected for catalog entries",
			serverUserType: types.ServerUserTypeMultiUser,
			expectError:    true,
		},
		{
			name:           "unknown value is rejected",
			serverUserType: types.ServerUserType("unknown"),
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := baseManifest
			manifest.ServerUserType = tt.serverUserType
			err := ValidateCatalogEntryManifest(manifest)
			if tt.expectError && err == nil {
				t.Errorf("expected error for serverUserType=%q, got nil", tt.serverUserType)
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error for serverUserType=%q: %v", tt.serverUserType, err)
			}
		})
	}
}
