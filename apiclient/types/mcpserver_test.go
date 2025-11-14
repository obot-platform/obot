package types

import (
	"testing"
)

func TestMapCatalogEntryToServer_UVX(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test UVX Server",
		Description: "Test UVX server description",
		Runtime:     RuntimeUVX,
		UVXConfig: &UVXRuntimeConfig{
			Package: "test-package",
			Args:    []string{"--verbose"},
		},
	}

	result, err := MapCatalogEntryToServer(catalogEntry, "", false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Runtime != RuntimeUVX {
		t.Errorf("Expected runtime %s, got %s", RuntimeUVX, result.Runtime)
	}

	if result.UVXConfig == nil {
		t.Fatal("Expected UVXConfig to be populated")
	}

	if result.UVXConfig.Package != "test-package" {
		t.Errorf("Expected package 'test-package', got '%s'", result.UVXConfig.Package)
	}

	if len(result.UVXConfig.Args) != 1 || result.UVXConfig.Args[0] != "--verbose" {
		t.Errorf("Expected args ['--verbose'], got %v", result.UVXConfig.Args)
	}
}

func TestMapCatalogEntryToServer_NPX(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test NPX Server",
		Description: "Test NPX server description",
		Runtime:     RuntimeNPX,
		NPXConfig: &NPXRuntimeConfig{
			Package: "@test/package",
			Args:    []string{"--port", "3000"},
		},
	}

	result, err := MapCatalogEntryToServer(catalogEntry, "", false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Runtime != RuntimeNPX {
		t.Errorf("Expected runtime %s, got %s", RuntimeNPX, result.Runtime)
	}

	if result.NPXConfig == nil {
		t.Fatal("Expected NPXConfig to be populated")
	}

	if result.NPXConfig.Package != "@test/package" {
		t.Errorf("Expected package '@test/package', got '%s'", result.NPXConfig.Package)
	}
}

func TestMapCatalogEntryToServer_Containerized(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test Containerized Server",
		Description: "Test containerized server description",
		Runtime:     RuntimeContainerized,
		ContainerizedConfig: &ContainerizedRuntimeConfig{
			Image: "test/mcp-server:latest",
			Port:  8080,
			Path:  "/mcp",
		},
	}

	result, err := MapCatalogEntryToServer(catalogEntry, "", false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Runtime != RuntimeContainerized {
		t.Errorf("Expected runtime %s, got %s", RuntimeContainerized, result.Runtime)
	}

	if result.ContainerizedConfig == nil {
		t.Fatal("Expected ContainerizedConfig to be populated")
	}

	if result.ContainerizedConfig.Image != "test/mcp-server:latest" {
		t.Errorf("Expected image 'test/mcp-server:latest', got '%s'", result.ContainerizedConfig.Image)
	}

	if result.ContainerizedConfig.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", result.ContainerizedConfig.Port)
	}

	if result.ContainerizedConfig.Path != "/mcp" {
		t.Errorf("Expected path '/mcp', got '%s'", result.ContainerizedConfig.Path)
	}
}

func TestMapCatalogEntryToServer_RemoteFixedURL(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test Remote Server",
		Description: "Test remote server description",
		Runtime:     RuntimeRemote,
		RemoteConfig: &RemoteCatalogConfig{
			FixedURL: "https://api.example.com/mcp",
		},
	}

	result, err := MapCatalogEntryToServer(catalogEntry, "", false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Runtime != RuntimeRemote {
		t.Errorf("Expected runtime %s, got %s", RuntimeRemote, result.Runtime)
	}

	if result.RemoteConfig == nil {
		t.Fatal("Expected RemoteConfig to be populated")
	}

	if result.RemoteConfig.URL != "https://api.example.com/mcp" {
		t.Errorf("Expected URL 'https://api.example.com/mcp', got '%s'", result.RemoteConfig.URL)
	}
}

func TestMapCatalogEntryToServer_RemoteHostname(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test Remote Server",
		Description: "Test remote server description",
		Runtime:     RuntimeRemote,
		RemoteConfig: &RemoteCatalogConfig{
			Hostname: "api.example.com",
		},
	}

	userURL := "https://api.example.com/custom/path"
	result, err := MapCatalogEntryToServer(catalogEntry, userURL, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Runtime != RuntimeRemote {
		t.Errorf("Expected runtime %s, got %s", RuntimeRemote, result.Runtime)
	}

	if result.RemoteConfig == nil {
		t.Fatal("Expected RemoteConfig to be populated")
	}

	if result.RemoteConfig.URL != userURL {
		t.Errorf("Expected URL '%s', got '%s'", userURL, result.RemoteConfig.URL)
	}
}

func TestMapCatalogEntryToServer_RemoteHostnameMismatch(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test Remote Server",
		Description: "Test remote server description",
		Runtime:     RuntimeRemote,
		RemoteConfig: &RemoteCatalogConfig{
			Hostname: "api.example.com",
		},
	}

	userURL := "https://wrong.example.com/custom/path"
	_, err := MapCatalogEntryToServer(catalogEntry, userURL, false)
	if err == nil {
		t.Fatal("Expected error for hostname mismatch")
	}

	validationErr, ok := err.(RuntimeValidationError)
	if !ok {
		t.Fatalf("Expected RuntimeValidationError, got %T", err)
	}

	if validationErr.Runtime != RuntimeRemote {
		t.Errorf("Expected runtime %s, got %s", RuntimeRemote, validationErr.Runtime)
	}

	if validationErr.Field != "userURL" {
		t.Errorf("Expected field 'userURL', got '%s'", validationErr.Field)
	}
}

func TestMapCatalogEntryToServer_MissingConfig(t *testing.T) {
	catalogEntry := MCPServerCatalogEntryManifest{
		Name:        "Test Server",
		Description: "Test server description",
		Runtime:     RuntimeUVX,
		// Missing UVXConfig
	}

	_, err := MapCatalogEntryToServer(catalogEntry, "", false)
	if err == nil {
		t.Fatal("Expected error for missing config")
	}

	validationErr, ok := err.(RuntimeValidationError)
	if !ok {
		t.Fatalf("Expected RuntimeValidationError, got %T", err)
	}

	if validationErr.Runtime != RuntimeUVX {
		t.Errorf("Expected runtime %s, got %s", RuntimeUVX, validationErr.Runtime)
	}
}

// Hostname validation tests

func TestValidateURLMatchesHostname(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		hostname    string
		expectError bool
	}{
		// Valid cases - exact hostname matches
		{
			name:        "exact hostname match",
			url:         "https://example.com/path",
			hostname:    "example.com",
			expectError: false,
		},
		{
			name:        "exact hostname match with port",
			url:         "https://example.com:8080/path",
			hostname:    "example.com",
			expectError: false,
		},
		{
			name:        "exact hostname match with subdomain",
			url:         "https://api.example.com/path",
			hostname:    "api.example.com",
			expectError: false,
		},
		{
			name:        "exact hostname match with http",
			url:         "http://example.com/path",
			hostname:    "example.com",
			expectError: false,
		},
		{
			name:        "exact hostname match with IP address",
			url:         "https://192.168.1.1/path",
			hostname:    "192.168.1.1",
			expectError: false,
		},
		{
			name:        "exact hostname match with localhost",
			url:         "http://localhost:3000/path",
			hostname:    "localhost",
			expectError: false,
		},

		// Valid cases - wildcard matches
		{
			name:        "wildcard match with single subdomain",
			url:         "https://api.example.com/path",
			hostname:    "*.example.com",
			expectError: false,
		},
		{
			name:        "wildcard match with multiple subdomains",
			url:         "https://api.v1.example.com/path",
			hostname:    "*.example.com",
			expectError: false,
		},
		{
			name:        "wildcard match with deep subdomain",
			url:         "https://foo.bar.baz.example.com/path",
			hostname:    "*.example.com",
			expectError: false,
		},
		{
			name:        "wildcard match with port",
			url:         "https://api.example.com:8080/path",
			hostname:    "*.example.com",
			expectError: false,
		},

		// Invalid cases - exact hostname mismatches
		{
			name:        "exact hostname mismatch",
			url:         "https://example.com/path",
			hostname:    "different.com",
			expectError: true,
		},
		{
			name:        "exact hostname mismatch with subdomain",
			url:         "https://api.example.com/path",
			hostname:    "example.com",
			expectError: true,
		},
		{
			name:        "exact hostname mismatch case sensitive",
			url:         "https://Example.com/path",
			hostname:    "example.com",
			expectError: true,
		},

		// Invalid cases - wildcard mismatches
		{
			name:        "wildcard mismatch - base domain doesn't match wildcard",
			url:         "https://example.com/path",
			hostname:    "*.example.com",
			expectError: true,
		},
		{
			name:        "wildcard mismatch - different domain",
			url:         "https://api.different.com/path",
			hostname:    "*.example.com",
			expectError: true,
		},
		{
			name:        "wildcard mismatch - partial domain match",
			url:         "https://api.notexample.com/path",
			hostname:    "*.example.com",
			expectError: true,
		},

		// Error cases - empty inputs
		{
			name:        "empty url",
			url:         "",
			hostname:    "example.com",
			expectError: true,
		},
		{
			name:        "empty hostname",
			url:         "https://example.com/path",
			hostname:    "",
			expectError: true,
		},
		{
			name:        "both empty",
			url:         "",
			hostname:    "",
			expectError: true,
		},

		// Error cases - invalid URLs
		{
			name:        "invalid url - malformed",
			url:         "not-a-valid-url",
			hostname:    "example.com",
			expectError: true,
		},
		{
			name:        "invalid url - missing scheme",
			url:         "example.com/path",
			hostname:    "example.com",
			expectError: true,
		},
		{
			name:        "url without hostname - file scheme",
			url:         "file:///path/to/file",
			hostname:    "example.com",
			expectError: true,
		},
		{
			name:        "url without hostname - relative",
			url:         "/path/only",
			hostname:    "example.com",
			expectError: true,
		},

		// Edge cases
		{
			name:        "url with query parameters",
			url:         "https://api.example.com/path?param=value",
			hostname:    "*.example.com",
			expectError: false,
		},
		{
			name:        "url with fragment",
			url:         "https://api.example.com/path#section",
			hostname:    "*.example.com",
			expectError: false,
		},
		{
			name:        "url with userinfo",
			url:         "https://user:pass@api.example.com/path",
			hostname:    "*.example.com",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURLHostname(tt.url, tt.hostname)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
		})
	}
}
