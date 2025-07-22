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

	result, err := MapCatalogEntryToServer(catalogEntry, "")
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

	result, err := MapCatalogEntryToServer(catalogEntry, "")
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

	result, err := MapCatalogEntryToServer(catalogEntry, "")
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

	result, err := MapCatalogEntryToServer(catalogEntry, "")
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
	result, err := MapCatalogEntryToServer(catalogEntry, userURL)
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
	_, err := MapCatalogEntryToServer(catalogEntry, userURL)
	if err == nil {
		t.Fatal("Expected error for hostname mismatch")
	}

	validationErr, ok := err.(RuntimeValidationError)
	if !ok {
		t.Fatalf("Expected RuntimeValidationError, got %T", err)
	}

	if validationErr.Runtime != string(RuntimeRemote) {
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

	_, err := MapCatalogEntryToServer(catalogEntry, "")
	if err == nil {
		t.Fatal("Expected error for missing config")
	}

	validationErr, ok := err.(RuntimeValidationError)
	if !ok {
		t.Fatalf("Expected RuntimeValidationError, got %T", err)
	}

	if validationErr.Runtime != string(RuntimeUVX) {
		t.Errorf("Expected runtime %s, got %s", RuntimeUVX, validationErr.Runtime)
	}
}

func TestValidateURLHostname(t *testing.T) {
	tests := []struct {
		name             string
		userURL          string
		requiredHostname string
		expectError      bool
	}{
		{
			name:             "Valid hostname match",
			userURL:          "https://api.example.com/path",
			requiredHostname: "api.example.com",
			expectError:      false,
		},
		{
			name:             "Hostname mismatch",
			userURL:          "https://wrong.example.com/path",
			requiredHostname: "api.example.com",
			expectError:      true,
		},
		{
			name:             "Invalid URL format",
			userURL:          "not-a-url",
			requiredHostname: "api.example.com",
			expectError:      true,
		},
		{
			name:             "Different port same hostname",
			userURL:          "https://api.example.com:8080/path",
			requiredHostname: "api.example.com",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURLHostname(tt.userURL, tt.requiredHostname)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
