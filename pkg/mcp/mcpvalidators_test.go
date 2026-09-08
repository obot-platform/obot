package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestValidateServerManifestForCatalog_MultiUserConfig(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "test-server",
		},
	}

	require.NoError(t, ValidateServerManifest(t.Context(), manifest, true, ValidationOptions{}))

	manifest.Config = []types.MCPConfig{{Usage: types.Header, UserAllowed: true, Key: "X-Tenant"}}
	require.Equal(t, types.RuntimeValidationError{
		Runtime: types.RuntimeNPX,
		Field:   "config",
		Message: "userAllowed may only be set for multi-user headers",
	}, ValidateServerManifest(t.Context(), manifest, false, ValidationOptions{}))
	require.NoError(t, ValidateServerManifest(t.Context(), manifest, true, ValidationOptions{}))
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
				FixedURL: "https://8.8.8.8/mcp",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with http",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://8.8.8.8/mcp",
			},
			expectError: false,
		},
		{
			name: "invalid fixedURL with localhost hostname",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://localhost:3000/mcp",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "localhost URL",
		},
		{
			name: "valid fixedURL with port",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://8.8.8.8:8080/mcp",
			},
			expectError: false,
		},
		{
			name: "valid fixedURL with path and query",
			config: types.RemoteCatalogConfig{
				FixedURL: "https://8.8.8.8/mcp/endpoint?param=value",
			},
			expectError: false,
		},
		{
			name: "invalid fixedURL with private IP address",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://192.168.1.1:8080/mcp",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "private IP address",
		},
		{
			name: "invalid fixedURL with link-local IP address",
			config: types.RemoteCatalogConfig{
				FixedURL: "http://169.254.169.254/latest/meta-data",
			},
			expectError: true,
			errorField:  "fixedURL",
			errorMsg:    "link-local address",
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

		// Valid cases - URLTemplate only
		{
			name: "valid urlTemplate with single variable",
			config: types.RemoteCatalogConfig{
				URLTemplate: "https://${API_HOST}/mcp/endpoint",
			},
			expectError: false,
		},
		{
			name: "tunnel with urlTemplate is invalid",
			config: types.RemoteCatalogConfig{
				TunnelName:  "mcptunnel-office",
				URLTemplate: "https://${API_HOST}/mcp/endpoint",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "tunnelName cannot be used with urlTemplate",
		},
		{
			name: "tunnel with templated fixedURL is invalid",
			config: types.RemoteCatalogConfig{
				TunnelName: "mcptunnel-office",
				FixedURL:   "https://${API_HOST}/mcp/endpoint",
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "tunnelName cannot be used with a URL template",
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
			err := validator.validateRemoteCatalogConfig(t.Context(), tt.config)

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
		{
			name:        "valid single letter domain",
			hostname:    "a.b",
			expectError: false,
		},
		{
			name:        "valid numbers only",
			hostname:    "123.456",
			expectError: false,
		},
		{
			name:        "valid mixed alphanumeric",
			hostname:    "a1b2.c3d4",
			expectError: false,
		},
		{
			name:        "valid long hostname",
			hostname:    "very-long-subdomain-name.very-long-domain-name.com",
			expectError: false,
		},
		{
			name:        "valid wildcard with single char",
			hostname:    "*.a",
			expectError: false,
		},
		{
			name:        "valid deep subdomain",
			hostname:    "a.b.c.d.e.f.g.h",
			expectError: false,
		},

		// Invalid cases for regex
		{
			name:        "empty string",
			hostname:    "",
			expectError: true,
		},
		{
			name:        "just wildcard",
			hostname:    "*",
			expectError: true,
		},
		{
			name:        "just dot",
			hostname:    ".",
			expectError: true,
		},
		{
			name:        "starts with dot",
			hostname:    ".example.com",
			expectError: true,
		},
		{
			name:        "ends with dot",
			hostname:    "example.com.",
			expectError: true,
		},
		{
			name:        "consecutive dots",
			hostname:    "example..com",
			expectError: true,
		},
		{
			name:        "wildcard not at start",
			hostname:    "sub.*.example.com",
			expectError: true,
		},
		{
			name:        "multiple wildcards",
			hostname:    "*.*.example.com",
			expectError: true,
		},
		{
			name:        "wildcard without dot",
			hostname:    "*example.com",
			expectError: true,
		},
		{
			name:        "contains slash",
			hostname:    "example.com/path",
			expectError: true,
		},
		{
			name:        "contains colon",
			hostname:    "example.com:8080",
			expectError: true,
		},
		{
			name:        "contains question mark",
			hostname:    "example.com?query",
			expectError: true,
		},
		{
			name:        "contains hash",
			hostname:    "example.com#fragment",
			expectError: true,
		},
		{
			name:        "contains at sign",
			hostname:    "user@example.com",
			expectError: true,
		},
		{
			name:        "contains space",
			hostname:    "example .com",
			expectError: true,
		},
		{
			name:        "contains tab",
			hostname:    "example\t.com",
			expectError: true,
		},
		{
			name:        "contains newline",
			hostname:    "example\n.com",
			expectError: true,
		},
		{
			name:        "unicode characters",
			hostname:    "exämple.com",
			expectError: true,
		},
		{
			name:        "chinese characters",
			hostname:    "例え.com",
			expectError: true,
		},
	}

	for _, tt := range regexTests {
		t.Run(tt.name, func(t *testing.T) {
			config := types.RemoteCatalogConfig{
				Hostname: tt.hostname,
			}

			err := validator.validateRemoteCatalogConfig(t.Context(), config)

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
				},
				Config: []types.MCPConfig{
					{Usage: types.Header, Key: "Authorization", Value: "Bearer token"},
					{Usage: types.Header, Key: "Content-Type", Value: "application/json"},
				},
			},
			expectError: false,
		},
		{
			name: "tunnel with URL template is invalid",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					TunnelName:  "mcptunnel-office",
					IsTemplate:  true,
					URLTemplate: "https://${API_HOST}/mcp",
				},
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "tunnelName cannot be used with a URL template",
		},
		{
			name: "tunnel with templated URL is invalid",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL:        "https://${API_HOST}/mcp",
					TunnelName: "mcptunnel-office",
				},
			},
			expectError: true,
			errorField:  "remoteConfig",
			errorMsg:    "tunnelName cannot be used with a URL template",
		},
		{
			name: "empty header key should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{Usage: types.Header, Key: "", Value: "some-value"},
				},
			},
			expectError: true,
			errorField:  "config[0].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "whitespace-only header key should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{Usage: types.Header, Key: "   ", Value: "some-value"},
				},
			},
			expectError: true,
			errorField:  "config[0].key",
			errorMsg:    "header key cannot be empty",
		},
		{
			name: "static header can be sensitive",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{Usage: types.Header, Key: "Authorization", Value: "Bearer token", Sensitive: true},
				},
			},
		},
		{
			name: "user-configurable header can be sensitive",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{Usage: types.Header, Key: "API-Key", Value: "", Sensitive: true, Required: true},
				},
			},
			expectError: false,
		},
		{
			name: "localhost URL should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "http://localhost:3000/mcp",
				},
			},
			expectError: true,
			errorField:  "url",
			errorMsg:    "localhost URL",
		},
		{
			name: "private IP URL should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "http://10.0.0.1:8080/mcp",
				},
			},
			expectError: true,
			errorField:  "url",
			errorMsg:    "private IP address",
		},
		{
			name: "link-local URL should fail",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "http://169.254.169.254/latest/meta-data",
				},
			},
			expectError: true,
			errorField:  "url",
			errorMsg:    "link-local address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateConfig(t.Context(), tt.manifest)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}

				if runtimeErr, ok := errors.AsType[types.RuntimeValidationError](err); ok {
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

func TestValidateRemoteManifestURLWithOptions(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		tunnelName string
		options    ValidationOptions
		wantErr    string
	}{
		{
			name:    "default rejects localhost",
			rawURL:  "http://localhost:8080/mcp",
			wantErr: "localhost URL",
		},
		{
			name:   "localhost allowed",
			rawURL: "http://localhost:8080/mcp",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowLocalhostMCP: true,
				},
			},
		},
		{
			name:    "default rejects loopback IP",
			rawURL:  "http://127.0.0.1:8080/mcp",
			wantErr: "localhost URL",
		},
		{
			name:   "loopback IP allowed by localhost option",
			rawURL: "http://127.0.0.1:8080/mcp",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowLocalhostMCP: true,
				},
			},
		},
		{
			name:    "default rejects private IP",
			rawURL:  "http://10.0.0.1:8080/mcp",
			wantErr: "private IP address",
		},
		{
			name:   "private IP allowed",
			rawURL: "http://10.0.0.1:8080/mcp",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowPrivateIPMCP: true,
				},
			},
		},
		{
			name:    "default rejects link-local IP",
			rawURL:  "http://169.254.169.254/latest/meta-data",
			wantErr: "link-local address",
		},
		{
			name:   "link-local IP allowed",
			rawURL: "http://169.254.169.254/latest/meta-data",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowLinkLocalMCP: true,
				},
			},
		},
		{
			name:    "private option does not allow link-local IP",
			rawURL:  "http://169.254.169.254/latest/meta-data",
			wantErr: "link-local address",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowPrivateIPMCP: true,
				},
			},
		},
		{
			name:    "link-local option does not allow private IP",
			rawURL:  "http://10.0.0.1:8080/mcp",
			wantErr: "private IP address",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowLinkLocalMCP: true,
				},
			},
		},
		{
			name:   "all URL validation blocks allowed",
			rawURL: "http://10.0.0.1:8080/mcp",
			options: ValidationOptions{
				RemoteMCPURLValidationConfig: RemoteMCPURLValidationConfig{
					AllowLocalhostMCP: true,
					AllowPrivateIPMCP: true,
					AllowLinkLocalMCP: true,
				},
			},
		},
		{
			name:       "tunnel permits ordinary private target URL",
			rawURL:     "https://10.0.0.1:8443/mcp",
			tunnelName: "mcptunnel-office",
		},
		{
			name:       "tunnel requires target hostname",
			rawURL:     "https:///mcp",
			tunnelName: "mcptunnel-office",
			wantErr:    "URL hostname is required",
		},
		{
			name:       "tunnel rejects target user information",
			rawURL:     "https://user@example.com/mcp",
			tunnelName: "mcptunnel-office",
			wantErr:    "URL must not include user information",
		},
		{
			name:    "legacy tunnel URL scheme is rejected",
			rawURL:  "https+tunnel://office:secret@10.0.0.1/mcp",
			wantErr: "URL scheme must be either https or http",
		},
	}

	validateServerManifest := func(ctx context.Context, rawURL, tunnelName string, options ValidationOptions) error {
		return ValidateServerManifest(ctx, types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{
				URL:        rawURL,
				TunnelName: tunnelName,
			},
		}, false, options)
	}

	validateCatalogEntryManifest := func(ctx context.Context, rawURL, tunnelName string, options ValidationOptions) error {
		return ValidateCatalogEntryManifest(ctx, types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{
				FixedURL:   rawURL,
				TunnelName: tunnelName,
			},
		}, false, options)
	}

	validateSystemManifest := func(ctx context.Context, rawURL, tunnelName string, options ValidationOptions) error {
		return ValidateSystemMCPServerManifest(ctx, types.SystemMCPServerManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{
				URL:        rawURL,
				TunnelName: tunnelName,
			},
		}, options)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, validator := range []struct {
				name string
				fn   func(context.Context, string, string, ValidationOptions) error
			}{
				{
					name: "server",
					fn:   validateServerManifest,
				},
				{
					name: "catalog entry",
					fn:   validateCatalogEntryManifest,
				},
				{
					name: "system server",
					fn:   validateSystemManifest,
				},
			} {
				t.Run(validator.name, func(t *testing.T) {
					err := validator.fn(t.Context(), tt.rawURL, tt.tunnelName, tt.options)
					if validator.name == "system server" && tt.tunnelName != "" {
						require.ErrorContains(t, err, "tunnels are not supported for system MCP servers")
						return
					}
					if tt.wantErr == "" {
						require.NoError(t, err)
						return
					}
					require.ErrorContains(t, err, tt.wantErr)
				})
			}
		})
	}
}

func TestValidateRemoteManifestAllowMissingURL(t *testing.T) {
	tests := []struct {
		name    string
		config  types.RemoteRuntimeConfig
		fields  []types.MCPConfig
		options ValidationOptions
		wantErr string
	}{
		{
			name:    "missing URL rejected by default",
			wantErr: "URL field cannot be empty",
		},
		{
			name:    "missing URL allowed",
			options: ValidationOptions{AllowMissingURL: true},
		},
		{
			name:    "whitespace URL allowed",
			config:  types.RemoteRuntimeConfig{URL: " \t "},
			options: ValidationOptions{AllowMissingURL: true},
		},
		{
			name:   "template may omit URL by default",
			config: types.RemoteRuntimeConfig{IsTemplate: true},
		},
		{
			name:    "present URL still requires HTTP scheme",
			config:  types.RemoteRuntimeConfig{URL: "ftp://example.com/mcp"},
			options: ValidationOptions{AllowMissingURL: true},
			wantErr: "URL scheme must be either https or http",
		},
		{
			name:    "present URL still receives remote URL validation",
			config:  types.RemoteRuntimeConfig{URL: "http://localhost:8080/mcp"},
			options: ValidationOptions{AllowMissingURL: true},
			wantErr: "localhost URL",
		},
		{
			name:    "missing URL still validates headers",
			fields:  []types.MCPConfig{{Usage: types.Header, Value: "value"}},
			options: ValidationOptions{AllowMissingURL: true},
			wantErr: "header key cannot be empty",
		},
		{
			name: "template without URL still validates headers",
			config: types.RemoteRuntimeConfig{
				IsTemplate: true,
			},
			fields:  []types.MCPConfig{{Usage: types.Header, Value: "value"}},
			wantErr: "header key cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &tt.config,
				Config:       tt.fields,
			}, false, tt.options)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateSystemCatalogEntryRejectsTunnel(t *testing.T) {
	err := ValidateSystemMCPServerCatalogEntryManifest(t.Context(), types.SystemMCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL:   "https://example.com/mcp",
			TunnelName: "mt1office",
		},
	}, ValidationOptions{})
	require.ErrorContains(t, err, "tunnels are not supported for system MCP servers")
}

func TestValidateSystemCatalogEntryRejectsInvalidConfigUsage(t *testing.T) {
	err := ValidateSystemMCPServerCatalogEntryManifest(t.Context(), types.SystemMCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "test-server",
		},
		Config: []types.MCPConfig{{
			Key:   "TOKEN",
			Usage: "invalid",
		}},
	}, ValidationOptions{})
	require.ErrorContains(t, err, `invalid usage "invalid" for config key "TOKEN"`)
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

func TestValidateManifestStartupTimeoutNonNegative(t *testing.T) {
	t.Run("server manifest rejects negative startup timeout", func(t *testing.T) {
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime: types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: -1,
			},
		}, false, ValidationOptions{})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig.startupTimeoutSeconds",
			Message: "must be greater than or equal to 0",
		}, err)
	})

	t.Run("catalog manifest rejects negative startup timeout", func(t *testing.T) {
		err := ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeUVX,
			UVXConfig: &types.UVXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: -1,
			},
		}, false, ValidationOptions{})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeUVX,
			Field:   "uvxConfig.startupTimeoutSeconds",
			Message: "must be greater than or equal to 0",
		}, err)
	})

	t.Run("server manifest rejects startup timeout above maximum", func(t *testing.T) {
		maxStartupTimeoutSeconds := int(MaxMCPServerStartupTimeout.Seconds())
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime: types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image:                 "test-image",
				Port:                  8080,
				Path:                  "/mcp",
				StartupTimeoutSeconds: maxStartupTimeoutSeconds + 1,
			},
		}, false, ValidationOptions{})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeContainerized,
			Field:   "containerizedConfig.startupTimeoutSeconds",
			Message: fmt.Sprintf("must be less than %d", maxStartupTimeoutSeconds),
		}, err)
	})

	t.Run("catalog manifest rejects startup timeout above maximum", func(t *testing.T) {
		maxStartupTimeoutSeconds := int(MaxMCPServerStartupTimeout.Seconds())
		err := ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{
				Package:               "test-package",
				StartupTimeoutSeconds: maxStartupTimeoutSeconds + 1,
			},
		}, false, ValidationOptions{})

		require.Equal(t, types.RuntimeValidationError{
			Runtime: types.RuntimeNPX,
			Field:   "npxConfig.startupTimeoutSeconds",
			Message: fmt.Sprintf("must be less than %d", maxStartupTimeoutSeconds),
		}, err)
	})
}

func TestValidateMCPResourceRequirements(t *testing.T) {
	validResources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{
			CPU:    "250m",
			Memory: "512Mi",
		},
		Limits: types.MCPResourceRequests{
			CPU:    "1",
			Memory: "1Gi",
		},
	}

	t.Run("server manifest accepts valid resources", func(t *testing.T) {
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
			Resources: validResources,
		}, false, ValidationOptions{})
		require.NoError(t, err)
	})

	t.Run("catalog manifest accepts valid resources", func(t *testing.T) {
		err := ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{
			Runtime:   types.RuntimeUVX,
			UVXConfig: &types.UVXRuntimeConfig{Package: "test-package"},
			Resources: validResources,
		}, false, ValidationOptions{})
		require.NoError(t, err)
	})

	tests := []struct {
		name        string
		resources   *types.MCPResourceRequirements
		field       string
		messagePart string
	}{
		{
			name: "invalid cpu request",
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{CPU: "not-cpu"},
			},
			field:       "resources.requests.cpu",
			messagePart: "invalid quantity",
		},
		{
			name: "invalid memory request",
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{Memory: "not-memory"},
			},
			field:       "resources.requests.memory",
			messagePart: "invalid quantity",
		},
		{
			name: "invalid cpu limit",
			resources: &types.MCPResourceRequirements{
				Limits: types.MCPResourceRequests{CPU: "not-cpu"},
			},
			field:       "resources.limits.cpu",
			messagePart: "invalid quantity",
		},
		{
			name: "invalid memory limit",
			resources: &types.MCPResourceRequirements{
				Limits: types.MCPResourceRequests{Memory: "not-memory"},
			},
			field:       "resources.limits.memory",
			messagePart: "invalid quantity",
		},
		{
			name: "negative cpu request",
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{CPU: "-250m"},
			},
			field:       "resources.requests.cpu",
			messagePart: "must be non-negative",
		},
		{
			name: "negative memory limit",
			resources: &types.MCPResourceRequirements{
				Limits: types.MCPResourceRequests{Memory: "-1Gi"},
			},
			field:       "resources.limits.memory",
			messagePart: "must be non-negative",
		},
		{
			name: "cpu limit below request",
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{CPU: "1"},
				Limits:   types.MCPResourceRequests{CPU: "500m"},
			},
			field:       "resources.limits.cpu",
			messagePart: "must be greater than or equal to resources.requests.cpu",
		},
		{
			name: "memory limit below request",
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{Memory: "1Gi"},
				Limits:   types.MCPResourceRequests{Memory: "512Mi"},
			},
			field:       "resources.limits.memory",
			messagePart: "must be greater than or equal to resources.requests.memory",
		},
	}

	for _, tt := range tests {
		t.Run("server manifest rejects "+tt.name, func(t *testing.T) {
			err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
				Resources: tt.resources,
			}, false, ValidationOptions{})

			var validationErr types.RuntimeValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, types.RuntimeNPX, validationErr.Runtime)
			require.Equal(t, tt.field, validationErr.Field)
			require.Contains(t, validationErr.Message, tt.messagePart)
		})

		t.Run("catalog manifest rejects "+tt.name, func(t *testing.T) {
			err := ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "test-package"},
				Resources: tt.resources,
			}, false, ValidationOptions{})

			var validationErr types.RuntimeValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, types.RuntimeUVX, validationErr.Runtime)
			require.Equal(t, tt.field, validationErr.Field)
			require.Contains(t, validationErr.Message, tt.messagePart)
		})
	}
}

func TestValidateMCPResourceMaximums(t *testing.T) {
	maxCPURequest := resource.MustParse("100m")
	options := ValidationOptions{
		ResourceMaximums: ResourceMaximums{
			CPURequest: &maxCPURequest,
		},
	}

	t.Run("server manifest rejects resources above maximum", func(t *testing.T) {
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
			Resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU: "250m",
				},
			},
		}, false, options)
		require.ErrorContains(t, err, "resources.requests.cpu 250m exceeds configured maximum 100m")
	})

	t.Run("server manifest accepts resources below maximum", func(t *testing.T) {
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
			Resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU: "50m",
				},
			},
		}, false, options)
		require.NoError(t, err)
	})

	t.Run("server manifest skips empty maximums", func(t *testing.T) {
		err := ValidateServerManifest(t.Context(), types.MCPServerManifest{
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
			Resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU: "250m",
				},
			},
		}, false, ValidationOptions{})
		require.NoError(t, err)
	})

	t.Run("catalog manifest rejects resources above maximum", func(t *testing.T) {
		err := ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{
			Runtime:   types.RuntimeUVX,
			UVXConfig: &types.UVXRuntimeConfig{Package: "test-package"},
			Resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU: "250m",
				},
			},
		}, false, options)
		require.ErrorContains(t, err, "resources.requests.cpu 250m exceeds configured maximum 100m")
	})
}

func TestRejectCompositeRuntime(t *testing.T) {
	manifest := types.MCPServerManifest{Runtime: types.RuntimeComposite}
	require.ErrorContains(t, ValidateServerManifest(t.Context(), manifest, false, ValidationOptions{}), "unsupported runtime")
	require.ErrorContains(t, ValidateCatalogEntryManifest(t.Context(), types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeComposite}, false, ValidationOptions{}), "unsupported runtime")
	require.ErrorContains(t, ValidateSystemMCPServerManifest(t.Context(), types.SystemMCPServerManifest{Runtime: types.RuntimeComposite}, ValidationOptions{}), "unsupported runtime")
}

func TestValidateSecretBindings(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "datadog-prod", Key: "api-key"}

	tests := []struct {
		name         string
		manifest     types.MCPServerManifest
		gitManaged   bool
		adminManaged bool
		backend      string
		wantErr      string // substring; "" = expect no error
	}{
		{
			name: "no bindings is allowed regardless",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "X-Foo", Value: "bar"}},
			},
			gitManaged: false,
			backend:    "docker",
		},
		{
			name: "bound header requires git-managed",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "DD-API-KEY", SecretBinding: binding}},
			},
			gitManaged: false,
			backend:    "kubernetes",
			wantErr:    "git-synced catalog entries",
		},
		{
			name: "bound header accepted for git-managed remote",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "DD-API-KEY", SecretBinding: binding}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "bound env accepted for admin-managed multi-user server",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				Config:  []types.MCPConfig{{Usage: types.Env, Key: "DD_API_KEY", SecretBinding: binding}},
			},
			adminManaged: true,
			backend:      "kubernetes",
		},
		{
			name: "bound multi-user header is rejected",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				Config: []types.MCPConfig{{
					Key: "X-API-Key", Usage: types.Header, UserAllowed: true, SecretBinding: binding,
				}},
			},
			adminManaged: true,
			backend:      "kubernetes",
			wantErr:      "secretBinding is not supported for user-defined headers",
		},
		{
			name: "bound header rejected on non-kubernetes backend",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "DD-API-KEY", SecretBinding: binding}},
			},
			gitManaged: true,
			backend:    "docker",
			wantErr:    "requires the kubernetes MCP runtime backend",
		},
		{
			name: "binding and static value are mutually exclusive",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "DD-API-KEY", Value: "literal", SecretBinding: binding}},
			},
			gitManaged: true,
			backend:    "kubernetes",
			wantErr:    "mutually exclusive",
		},
		{
			name: "binding requires non-empty name/key",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{},
				Config:       []types.MCPConfig{{Usage: types.Header, Key: "DD-API-KEY", SecretBinding: &types.MCPSecretBinding{Name: "datadog-prod"}}},
			},
			gitManaged: true,
			backend:    "kubernetes",
			wantErr:    "requires both name and key",
		},
		{
			name: "bound env under remote runtime is rejected",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				Config:       []types.MCPConfig{{Usage: types.Env, Key: "DD_API_KEY", SecretBinding: binding}},
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
				Config:  []types.MCPConfig{{Key: "DD_API_KEY", SecretBinding: binding, Usage: types.File}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "bound env accepted for git-managed containerized",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				Config:  []types.MCPConfig{{Usage: types.Env, Key: "DD_API_KEY", SecretBinding: binding}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "env binding is accepted",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{Usage: types.Env,
					Key: "DD_API_KEY", SecretBinding: binding,
				}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
		{
			name: "dynamicFile is accepted",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{
					Key:   "DD_API_KEY",
					Usage: types.DynamicFile,
				}},
			},
			gitManaged: true,
			backend:    "kubernetes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindings(tt.manifest, tt.gitManaged, tt.adminManaged, tt.backend)
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
				Config:  []types.MCPConfig{{Key: "HOST", Usage: types.Env, Required: true}},
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: "https://${HOST}/mcp",
				},
			},
		},
		{
			name: "urlTemplate referencing secret-bound env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, Required: true, SecretBinding: binding}},
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
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, Required: true, SecretBinding: binding}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindingsCatalogEntry(tt.manifest, true, false, "kubernetes")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateSecretBindingsCatalogEntryAdminManaged(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "my-secret", Key: "token"}

	tests := []struct {
		name         string
		manifest     types.MCPServerCatalogEntryManifest
		adminManaged bool
		wantErr      string
	}{
		{
			name: "admin-managed non-git catalog entry allows env binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, SecretBinding: binding}},
			},
			adminManaged: true,
		},
		{
			name: "non-admin non-git catalog entry rejects env binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, SecretBinding: binding}},
			},
			wantErr: "administrator-managed vMCPs",
		},
		{
			name: "admin-managed non-git catalog entry allows env binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, SecretBinding: binding}},
			},
			adminManaged: true,
		},
		{
			name: "admin-managed non-git remote catalog entry allows header binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Config:  []types.MCPConfig{{Key: "Authorization", Usage: types.Header, SecretBinding: binding}},
			},
			adminManaged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindingsCatalogEntry(tt.manifest, false, tt.adminManaged, "kubernetes")
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateSecretBindingsCatalogEntryRejectsAdminAdded(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "my-secret", Key: "token", AdminAdded: true}

	tests := []struct {
		name     string
		manifest types.MCPServerCatalogEntryManifest
		wantErr  string
	}{
		{
			name: "env config adminAdded rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config:  []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, SecretBinding: binding}},
			},
			wantErr: "secretBinding.adminAdded is not valid for catalog entry",
		},
		{
			name: "header config adminAdded rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				Config:  []types.MCPConfig{{Key: "Authorization", Usage: types.Header, SecretBinding: binding}},
			},
			wantErr: "secretBinding.adminAdded is not valid for catalog entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSecretBindingsCatalogEntry(tt.manifest, true, false, "kubernetes")
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateTemplateReferences_Server(t *testing.T) {
	required := types.MCPConfig{Usage: types.Env, Key: "TAG", Required: true}
	optional := types.MCPConfig{Usage: types.Env, Key: "TAG", Required: false}

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
				Config:    []types.MCPConfig{required},
			},
		},
		{
			name: "npx templated arg with optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Config:    []types.MCPConfig{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "uvx templated command with required env passes",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: "pkg", Command: "${TAG}"},
				Config:    []types.MCPConfig{required},
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
				Config: []types.MCPConfig{optional},
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
				Config: []types.MCPConfig{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote header value templated by optional env is rejected",
			manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{optional, {Usage: types.Header, Key: "Authorization", Value: "Bearer ${TAG}"}},
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
	required := types.MCPConfig{Key: "TAG", Usage: types.Interpolated, Required: true}
	optional := types.MCPConfig{Key: "TAG", Usage: types.Interpolated, Required: false}

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
				Config:    []types.MCPConfig{required},
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
				Config: []types.MCPConfig{required},
			},
		},
		{
			name: "npx templated arg with optional env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "pkg", Args: []string{"--tag=${TAG}"}},
				Config:    []types.MCPConfig{optional},
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
				Config: []types.MCPConfig{optional},
			},
			wantErr: "must be required=true",
		},
		{
			name: "remote header value templated by undeclared env is rejected",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
				Config:       []types.MCPConfig{{Key: "Authorization", Usage: types.Header, Value: "Bearer ${TAG}"}},
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

func TestValidateCatalogEntryManifest_ShortDescriptionMaxLength(t *testing.T) {
	base := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "test-server",
		},
		ShortDescription: strings.Repeat("a", maxShortDescriptionLength),
	}
	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), base, false, ValidationOptions{}))

	base.ShortDescription = strings.Repeat("a", maxShortDescriptionLength+1)
	err := ValidateCatalogEntryManifest(t.Context(), base, false, ValidationOptions{})
	require.ErrorContains(t, err, fmt.Sprintf("short description must be less than or equal to %d characters", maxShortDescriptionLength))
}

func TestValidateCatalogEntryManifestCatalogSyncedRejectsTunnelName(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{
		Name:    "Remote",
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL:   "https://example.com/mcp",
			TunnelName: "mt1office",
		},
	}

	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), manifest, false, ValidationOptions{}))
	require.Equal(t, types.RuntimeValidationError{
		Runtime: types.RuntimeRemote,
		Field:   "remoteConfig.tunnelName",
		Message: "cannot be set on catalog-synced entries",
	}, ValidateCatalogEntryManifest(t.Context(), manifest, true, ValidationOptions{}))
}
