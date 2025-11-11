package validation

import (
	"errors"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

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
					RemoteCatalogConfig: types.RemoteCatalogConfig{
						Headers: []types.MCPHeader{
							{Key: "Authorization", Value: "Bearer token"},
							{Key: "Content-Type", Value: "application/json"},
						},
					},
					URL: "https://example.com/mcp",
				},
			},
			expectError: false,
		},
		{
			name: "empty header key should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					RemoteCatalogConfig: types.RemoteCatalogConfig{
						Headers: []types.MCPHeader{
							{Key: "", Value: "some-value"},
						},
					},
					URL: "https://example.com/mcp",
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
					RemoteCatalogConfig: types.RemoteCatalogConfig{
						Headers: []types.MCPHeader{
							{Key: "   ", Value: "some-value"},
						},
					},
					URL: "https://example.com/mcp",
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
					RemoteCatalogConfig: types.RemoteCatalogConfig{
						Headers: []types.MCPHeader{
							{Key: "Authorization", Value: "Bearer token", Sensitive: true},
						},
					},
					URL: "https://example.com/mcp",
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
					RemoteCatalogConfig: types.RemoteCatalogConfig{
						Headers: []types.MCPHeader{
							{Key: "API-Key", Value: "", Sensitive: true, Required: true},
						},
					},
					URL: "https://example.com/mcp",
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
