package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic spaces",
			input:    "My App Config",
			expected: "my-app-config",
		},
		{
			name:     "single quotes and spaces",
			input:    "My App's Config",
			expected: "my-app-s-config",
		},
		{
			name:     "special characters",
			input:    "Test_Server@1.0!",
			expected: "test-server-1-0",
		},
		{
			name:     "mixed case with symbols",
			input:    "Special!@#$%Characters",
			expected: "special-characters",
		},
		{
			name:     "multiple consecutive spaces",
			input:    "App   With   Spaces",
			expected: "app-with-spaces",
		},
		{
			name:     "leading and trailing spaces",
			input:    "  App Config  ",
			expected: "app-config",
		},
		{
			name:     "leading and trailing special chars",
			input:    "!!!App Config***",
			expected: "app-config",
		},
		{
			name:     "only special characters",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "already valid name",
			input:    "my-valid-name",
			expected: "my-valid-name",
		},
		{
			name:     "numbers and hyphens",
			input:    "app-v1.2.3",
			expected: "app-v1-2-3",
		},
		{
			name:     "unicode characters",
			input:    "café-résumé",
			expected: "caf-r-sum",
		},
		{
			name:     "long name gets truncated",
			input:    "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-sixty-three-characters-and-should-be-truncated",
			expected: "this-is-a-very-long-name-that-exceeds-the-kubernetes-limit-of-s",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},
		{
			name:     "uppercase letters",
			input:    "UPPERCASE-NAME",
			expected: "uppercase-name",
		},
		{
			name:     "mixed alphanumeric with symbols",
			input:    "App123@#$Test456",
			expected: "app123-test456",
		},
		{
			name:     "parentheses and brackets",
			input:    "App (v2.0) [Production]",
			expected: "app-v2-0-production",
		},
		{
			name:     "dots and underscores",
			input:    "my.app_name.config",
			expected: "my-app-name-config",
		},
		{
			name:     "consecutive special chars become single dash",
			input:    "app!!!@@@###config",
			expected: "app-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeNameKubernetesCompliance(t *testing.T) {
	testCases := []string{
		"My App's Config",
		"Test_Server@1.0!",
		"Special!@#$%Characters",
		"App   With   Spaces",
		"  App Config  ",
		"café-résumé",
		"UPPERCASE-NAME",
		"App (v2.0) [Production]",
	}

	for _, input := range testCases {
		t.Run(input, func(t *testing.T) {
			result := normalizeMCPCatalogEntryName(input)

			// Test length constraint
			if len(result) > 63 {
				t.Errorf("NormalizeName(%q) = %q has length %d, exceeds 63 characters", input, result, len(result))
			}

			// Test character constraints (only lowercase alphanumeric and hyphens)
			for i, r := range result {
				if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
					t.Errorf("NormalizeName(%q) = %q contains invalid character %q at position %d", input, result, r, i)
				}
			}

			// Test that it doesn't start or end with hyphen (unless empty)
			if len(result) > 0 {
				if result[0] == '-' {
					t.Errorf("NormalizeName(%q) = %q starts with hyphen", input, result)
				}
				if result[len(result)-1] == '-' {
					t.Errorf("NormalizeName(%q) = %q ends with hyphen", input, result)
				}
			}
		})
	}
}

func newEntry(catalogName, workspaceID string) v1.MCPServerCatalogEntry {
	return v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName:       catalogName,
			PowerUserWorkspaceID: workspaceID,
		},
	}
}

func TestValidateEntryScope(t *testing.T) {
	tests := []struct {
		name        string
		entry       v1.MCPServerCatalogEntry
		catalogName string
		workspaceID string
		expectError bool
	}{
		{
			name:        "catalog entry matches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "default",
			expectError: false,
		},
		{
			name:        "catalog entry mismatches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "other",
			expectError: true,
		},
		{
			name:        "workspace entry matches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "workspace entry mismatches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws2",
			expectError: true,
		},
		{
			name:        "global catalog entry rejected by strict workspace check",
			entry:       newEntry("default", ""),
			workspaceID: "ws1",
			expectError: true,
		},
		{
			name:        "unscoped request for unscoped entry passes",
			entry:       newEntry("", ""),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEntryScope(tt.entry, tt.catalogName, tt.workspaceID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateEntryVisibleFromScope(t *testing.T) {
	tests := []struct {
		name        string
		entry       v1.MCPServerCatalogEntry
		catalogName string
		workspaceID string
		expectError bool
	}{
		{
			name:        "catalog entry matches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "default",
			expectError: false,
		},
		{
			name:        "catalog entry mismatches catalog scope",
			entry:       newEntry("default", ""),
			catalogName: "other",
			expectError: true,
		},
		{
			name:        "workspace entry matches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "workspace entry mismatches workspace scope",
			entry:       newEntry("", "ws1"),
			workspaceID: "ws2",
			expectError: true,
		},
		{
			name:        "global catalog entry allowed via workspace scope (relaxed)",
			entry:       newEntry("default", ""),
			workspaceID: "ws1",
			expectError: false,
		},
		{
			name:        "entry with no scope rejected via workspace scope",
			entry:       newEntry("", ""),
			workspaceID: "ws1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEntryVisibleFromScope(tt.entry, tt.catalogName, tt.workspaceID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPopulateComponentManifestsHydratesMCPServerID(t *testing.T) {
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-server", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "default",
			Manifest: types.MCPServerManifest{
				Name:            "Shared Server",
				Runtime:         types.RuntimeContainerized,
				MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "API_KEY", Name: "API Key"}}},
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "example/shared:1.0.0",
					Port:  8080,
					Path:  "/mcp",
				},
			},
		},
	}
	manifest := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{MCPServerID: "shared-server"},
		}},
	}

	err := (&MCPCatalogHandler{}).populateComponentManifests(newPopulateComponentManifestsRequest(server), &manifest, "default", "")

	require.NoError(t, err)
	require.Len(t, manifest.CompositeConfig.ComponentServers, 1)
	component := manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "shared-server", component.MCPServerID)
	assert.Empty(t, component.CatalogEntryID)
	assert.Equal(t, "Shared Server", component.Manifest.Name)
	assert.Equal(t, types.RuntimeContainerized, component.Manifest.Runtime)
	require.NotNil(t, component.Manifest.ContainerizedConfig)
	assert.Equal(t, "example/shared:1.0.0", component.Manifest.ContainerizedConfig.Image)
	require.NotNil(t, component.Manifest.MultiUserConfig)
}

func TestPopulateComponentManifestsHydratesSameCatalogEntryID(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "component-entry", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "custom",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:           "Component Server",
				Runtime:        types.RuntimeNPX,
				ServerUserType: types.ServerUserTypeSingleUser,
				NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/component"},
			},
		},
	}
	manifest := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "component-entry"},
		}},
	}

	err := (&MCPCatalogHandler{}).populateComponentManifests(newPopulateComponentManifestsRequest(entry), &manifest, "custom", "")

	require.NoError(t, err)
	require.Len(t, manifest.CompositeConfig.ComponentServers, 1)
	component := manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "component-entry", component.CatalogEntryID)
	assert.Empty(t, component.MCPServerID)
	assert.Equal(t, "Component Server", component.Manifest.Name)
	assert.Equal(t, types.RuntimeNPX, component.Manifest.Runtime)
	require.NotNil(t, component.Manifest.NPXConfig)
	assert.Equal(t, "@example/component", component.Manifest.NPXConfig.Package)
}

func newPopulateComponentManifestsRequest(objects ...client.Object) api.Context {
	return api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Storage: storage.Client(fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(objects...).
			Build()),
	}
}
