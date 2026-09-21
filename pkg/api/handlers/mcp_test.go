package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServerNeedsOAuthForPendingStaticOAuth(t *testing.T) {
	handler := MCPHandler{}
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					StaticOAuthRequired: true,
				},
			},
		},
	}

	needsOAuth, err := handler.serverNeedsOAuth(t.Context(), &server, mcp.ServerConfig{Runtime: types.RuntimeRemote})
	require.NoError(t, err)
	require.True(t, needsOAuth)

	server.Status.UserHasAuthenticated = true
	needsOAuth, err = handler.serverNeedsOAuth(t.Context(), &server, mcp.ServerConfig{Runtime: types.RuntimeUVX})
	require.NoError(t, err)
	require.False(t, needsOAuth)
}

func TestConvertMCPServer_StaticEnvIsConfigured(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{
					Key:      "CATALOG_TOKEN",
					Value:    "catalog-value",
					Required: true,
					Usage:    types.Env,
				}},
			},
		},
	}

	converted := ConvertMCPServer(server, nil, "", "")

	assert.True(t, converted.Configured)
	assert.Empty(t, converted.MissingRequiredEnvVars)
}

func TestConvertMCPResources(t *testing.T) {
	resources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{CPU: "250m", Memory: "512Mi"},
		Limits:   types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
	}

	entry := ConvertMCPServerCatalogEntry(v1.MCPServerCatalogEntry{
		Name: "entry",
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:      "entry",
				Resources: resources,
				Runtime:   types.RuntimeRemote,
			},
		},
	}, "https://example.com")
	assert.Equal(t, resources, entry.Manifest.Resources)
	assert.Equal(t, "https://example.com/mcp-connect/entry", entry.ConnectURL)

	server := ConvertMCPServer(v1.MCPServer{
		Name: "server",
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:      "server",
				Resources: resources,
			},
		},
	}, nil, "", "")
	assert.Equal(t, resources, server.MCPServerManifest.Resources)
}

func TestConvertMCPServerCatalogEntryDetached(t *testing.T) {
	entry := ConvertMCPServerCatalogEntry(v1.MCPServerCatalogEntry{
		Name: "entry",
		Spec: v1.MCPServerCatalogEntrySpec{
			Editable:  true,
			Detached:  true,
			SourceURL: "https://github.com/obot-platform/mcp-catalog",
			Manifest: types.MCPServerCatalogEntryManifest{
				UpgradeNote: "Review the new settings.",
			},
		},
	}, "https://example.com")

	assert.True(t, entry.Detached)
	assert.True(t, entry.Editable)
	assert.Equal(t, "https://github.com/obot-platform/mcp-catalog", entry.SourceURL)
	assert.Equal(t, "Review the new settings.", entry.Manifest.UpgradeNote)
}

func TestValidationOptionsWithResourceMaximumsIgnoresPersistedMaximumForNonKubernetesBackend(t *testing.T) {
	maximum := resource.MustParse("500m")
	req := api.Context{
		Request: httptest.NewRequest(http.MethodGet, "/", nil),
		Storage: newFakeStorage(t, &v1.K8sSettings{
			Name: system.K8sSettingsName, Namespace: system.DefaultNamespace,
			Spec: v1.K8sSettingsSpec{MaxCPURequest: &maximum},
		}),
	}

	options, err := ValidationOptionsWithResourceMaximums(req, &mcp.SessionManager{})
	require.NoError(t, err)
	require.Nil(t, options.ResourceMaximums.CPURequest)

	err = mcp.ValidateServerManifest(t.Context(), types.MCPServerManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "example",
		},
		Resources: &types.MCPResourceRequirements{
			Requests: types.MCPResourceRequests{CPU: "1"},
		},
	}, false, options)
	require.NoError(t, err)
}

func TestMCPServerOrInstanceFromConnectURLRejectsCatalogEntryResourcesAboveMaximum(t *testing.T) {
	entry := v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:    "entry",
				Runtime: types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{
					Package: "test-package",
				},
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{
						CPU: "250m",
					},
				},
			},
		},
	}
	storage := newFakeStorage(t, &entry)

	_, _, err := mcpServerOrInstanceFromConnectURL(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodGet, "/mcp-connect/entry", nil),
		Storage:        storage,
		User:           testUser("user"),
	}, "entry", "", mcp.ValidationOptions{
		ResourceMaximums: mcp.ResourceMaximums{
			CPURequest: new(resource.MustParse("100m")),
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resources.requests.cpu 250m exceeds configured maximum 100m")

	var servers v1.MCPServerList
	require.NoError(t, storage.List(t.Context(), &servers, kclient.InNamespace(system.DefaultNamespace)))
	assert.Empty(t, servers.Items)
}

// Test functions for applyURLTemplate
func TestApplyURLTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		envVars     map[string]string
		expected    string
		expectError bool
	}{
		{
			name:     "basic substitution",
			template: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
			envVars: map[string]string{
				"DATABRICKS_WORKSPACE_URL":  "workspace.cloud.databricks.com",
				"DATABRICKS_GENIE_SPACE_ID": "12345",
			},
			expected:    "https://workspace.cloud.databricks.com/api/2.0/mcp/genie/12345",
			expectError: false,
		},
		{
			name:     "single variable",
			template: "https://${API_HOST}/v1/endpoint",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
			},
			expected:    "https://api.example.com/v1/endpoint",
			expectError: false,
		},
		{
			name:        "no variables",
			template:    "https://example.com/api",
			envVars:     map[string]string{},
			expected:    "https://example.com/api",
			expectError: false,
		},
		{
			name:        "empty template",
			template:    "",
			envVars:     map[string]string{},
			expected:    "",
			expectError: false,
		},
		{
			name:     "variable with special characters",
			template: "https://${API_HOST}/path/${USER_ID}/data",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
				"USER_ID":  "user-123_456",
			},
			expected:    "https://api.example.com/path/user-123_456/data",
			expectError: false,
		},
		{
			name:     "multiple same variable",
			template: "https://${API_HOST}/api/${API_HOST}/status",
			envVars: map[string]string{
				"API_HOST": "api.example.com",
			},
			expected:    "https://api.example.com/api/api.example.com/status",
			expectError: false,
		},
		{
			name:     "variable in query string",
			template: "https://${API_HOST}/api?token=${API_TOKEN}&user=${USER_ID}",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"API_TOKEN": "abc123",
				"USER_ID":   "user456",
			},
			expected:    "https://api.example.com/api?token=abc123&user=user456",
			expectError: false,
		},
		{
			name:     "variable with empty value",
			template: "https://${API_HOST}/api/${EMPTY_VAR}/data",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"EMPTY_VAR": "",
			},
			expectError: true,
		},
		{
			name:     "variable with spaces",
			template: "https://${API_HOST}/api/${USER_NAME}/profile",
			envVars: map[string]string{
				"API_HOST":  "api.example.com",
				"USER_NAME": "John Doe",
			},
			expected:    "https://api.example.com/api/John Doe/profile",
			expectError: false,
		},
		{
			name:     "complex path with variables",
			template: "https://${REGION}.${SERVICE}.${PROVIDER}.com/${VERSION}/${RESOURCE}/${ID}",
			envVars: map[string]string{
				"REGION":   "us-west-2",
				"SERVICE":  "compute",
				"PROVIDER": "aws",
				"VERSION":  "v1",
				"RESOURCE": "instances",
				"ID":       "i-1234567890abcdef0",
			},
			expected:    "https://us-west-2.compute.aws.com/v1/instances/i-1234567890abcdef0",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, nil, tt.envVars)

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

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestApplyRemoteURLTemplate(t *testing.T) {
	manifest := types.MCPServerManifest{
		Name:    "OAuth Remote",
		Runtime: types.RuntimeRemote,

		RemoteConfig: &types.RemoteRuntimeConfig{
			IsTemplate:          true,
			URLTemplate:         "https://${HOST}/mcp/${API_VERSION}/projects/${PROJECT_ID}",
			StaticOAuthRequired: true,
		},
		Config: []types.MCPConfig{{
			Key:   "HOST",
			Value: "remote.example.com",
			Usage: types.Env,
		},

			{
				Key:   "API_VERSION",
				Value: "v1",
				Usage: types.Header,
			}},
	}
	options := mcp.ValidationOptions{
		RemoteMCPURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
			AllowLocalhostMCP: true,
			AllowPrivateIPMCP: true,
			AllowLinkLocalMCP: true,
		},
	}

	err := applyRemoteURLTemplate(t.Context(), &manifest, map[string]string{
		"PROJECT_ID": "project-123",
	}, false, options)
	require.NoError(t, err)
	require.Equal(t, "https://remote.example.com/mcp/v1/projects/project-123", manifest.RemoteConfig.URL)
	require.True(t, manifest.RemoteConfig.StaticOAuthRequired)

	server := v1.MCPServer{Name: "tool-preview", Spec: v1.MCPServerSpec{Manifest: manifest}}
	serverConfig, missing, err := mcp.ServerToServerConfig(server, nil, "system", "temp", "default", nil)
	require.NoError(t, err)
	require.Empty(t, missing)
	require.Equal(t, manifest.RemoteConfig.URL, serverConfig.URL)
}

func TestApplyURLTemplateStaticValuesOverrideSubmittedConfiguration(t *testing.T) {
	result, err := applyURLTemplate(
		"https://${HOST}/${VERSION}",
		[]types.MCPConfig{{
			Key:   "HOST",
			Value: "catalog.example.com",
			Usage: types.Env,
		}, {
			Key:   "VERSION",
			Value: "v1",
			Usage: types.Header,
		}},
		map[string]string{
			"HOST":    "forged.example.com",
			"VERSION": "forged",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "https://catalog.example.com/v1", result)
}

func TestValidateConfiguredOptionsExcludesPerUserHeaders(t *testing.T) {
	config := []types.MCPConfig{{
		Key:         "X-Region",
		Usage:       types.Header,
		UserAllowed: true,
		Required:    true,
		Options:     []types.MCPConfigurationOption{{Name: "East", Value: "east"}},
	}}
	require.NoError(t, validateConfiguredOptions(config, nil))
	missing, err := mcp.ValidateConfiguredOptions(config, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"X-Region"}, missing)
	server := v1.MCPServer{Spec: v1.MCPServerSpec{Manifest: types.MCPServerManifest{Config: config}}}
	require.Empty(t, ConvertMCPServer(server, nil, "", "").MissingRequiredHeaders)
}

func TestApplyRemoteURLTemplateRejectsInvalidRenderedURL(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{
			IsTemplate:  true,
			URLTemplate: "${SCHEME}://remote.example.com/mcp",
		},
	}

	err := applyRemoteURLTemplate(t.Context(), &manifest, map[string]string{"SCHEME": "ftp"}, false, mcp.ValidationOptions{})
	require.Error(t, err)
	require.ErrorContains(t, err, "URL scheme must be either https or http")
}

func TestApplyRemoteURLTemplateRejectsMissingConfiguration(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteRuntimeConfig{
			IsTemplate:  true,
			URLTemplate: "https://remote.example.com/mcp/${PROJECT_ID}",
		},
	}

	err := applyRemoteURLTemplate(t.Context(), &manifest, nil, false, mcp.ValidationOptions{})
	require.Error(t, err)
	var configErr *urlTemplateConfigurationError
	require.ErrorAs(t, err, &configErr)
	require.ErrorContains(t, err, `configuration value "PROJECT_ID" referenced by remoteConfig.urlTemplate is required`)
}

func TestApplyURLTemplateEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		envVars     map[string]string
		description string
		expected    string
		expectError bool
	}{
		{
			name:        "unmatched variable is rejected",
			template:    "https://${API_HOST}/api/${MISSING_VAR}/data",
			envVars:     map[string]string{"API_HOST": "api.example.com"},
			description: "Every referenced variable must resolve before the URL is used",
			expectError: true,
		},
		{
			name:        "case sensitive variables",
			template:    "https://${API_HOST}/api/${api_host}/data",
			envVars:     map[string]string{"API_HOST": "api.example.com", "api_host": "different.example.com"},
			description: "Variable names are case sensitive",
			expected:    "https://api.example.com/api/different.example.com/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, nil, tt.envVars)
			if tt.expectError {
				require.Error(t, err)
				var configErr *urlTemplateConfigurationError
				require.ErrorAs(t, err, &configErr)
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestApplyURLTemplatePerformance(t *testing.T) {
	// Test with a large number of variables
	largeEnvVars := make(map[string]string, 1000)
	for i := range 1000 {
		key := fmt.Sprintf("VAR_%d", i)
		value := fmt.Sprintf("value_%d", i)
		largeEnvVars[key] = value
	}

	var template strings.Builder
	template.WriteString("https://example.com/api")
	for i := range 100 {
		_, _ = fmt.Fprintf(&template, "/${VAR_%d}", i)
	}

	start := time.Now()
	result, err := applyURLTemplate(template.String(), nil, largeEnvVars)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	if result == "" {
		t.Errorf("expected non-empty result")
		return
	}

	// Performance should be reasonable (less than 100ms for 100 variables)
	if duration > 100*time.Millisecond {
		t.Errorf("performance test took too long: %v", duration)
	}

	t.Logf("Processed template with 100 variables in %v", duration)
}

func TestApplyURLTemplateRealWorldExamples(t *testing.T) {
	tests := []struct {
		name     string
		template string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "Databricks example",
			template: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
			envVars: map[string]string{
				"DATABRICKS_WORKSPACE_URL":  "workspace.cloud.databricks.com",
				"DATABRICKS_GENIE_SPACE_ID": "12345",
			},
			expected: "https://workspace.cloud.databricks.com/api/2.0/mcp/genie/12345",
		},
		{
			name:     "AWS API Gateway",
			template: "https://${API_ID}.execute-api.${REGION}.amazonaws.com/${STAGE}/${RESOURCE}",
			envVars: map[string]string{
				"API_ID":   "abc123def4",
				"REGION":   "us-east-1",
				"STAGE":    "prod",
				"RESOURCE": "users",
			},
			expected: "https://abc123def4.execute-api.us-east-1.amazonaws.com/prod/users",
		},
		{
			name:     "Google Cloud",
			template: "https://${PROJECT_ID}.${REGION}.run.app/${SERVICE_NAME}",
			envVars: map[string]string{
				"PROJECT_ID":   "my-project-123",
				"REGION":       "us-central1",
				"SERVICE_NAME": "api-service",
			},
			expected: "https://my-project-123.us-central1.run.app/api-service",
		},
		{
			name:     "Azure Functions",
			template: "https://${FUNCTION_APP}.azurewebsites.net/api/${FUNCTION_NAME}?code=${FUNCTION_KEY}",
			envVars: map[string]string{
				"FUNCTION_APP":  "my-function-app",
				"FUNCTION_NAME": "process-data",
				"FUNCTION_KEY":  "abc123def456",
			},
			expected: "https://my-function-app.azurewebsites.net/api/process-data?code=abc123def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := applyURLTemplate(tt.template, nil, tt.envVars)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCreateCatalogEntryAllowsConfigurationOptions(t *testing.T) {
	storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace})
	options := []types.MCPConfigurationOption{
		{Name: "United States", Value: "us", Description: "US endpoint"},
		{Name: "Europe", Value: "eu", Description: "EU endpoint"},
	}
	manifest := types.MCPServerCatalogEntryManifest{
		Name:      "option-entry",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config: []types.MCPConfig{{
			Key:      "REGION",
			Name:     "Region",
			Required: true,
			Options:  options,
			Usage:    types.Env,
		}},
	}
	body, err := json.Marshal(manifest)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp-catalogs/catalog-1/entries", bytes.NewReader(body))
	req.SetPathValue("catalog_id", "catalog-1")

	err = (&MCPCatalogHandler{mcpBackend: "docker", sessionManager: &mcp.SessionManager{}}).CreateEntry(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
		User:           testUserWithRole("admin", types.GroupAdmin),
	})

	require.NoError(t, err)
	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, storage.List(t.Context(), &entries))
	require.Len(t, entries.Items, 1)
	require.Len(t, entries.Items[0].Spec.Manifest.Config, 1)
	assert.Equal(t, options, entries.Items[0].Spec.Manifest.Config[0].Options)
}

func newCreateServerSecretBindingK8sClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestServerManifestFromCatalogEntryManifestAllowsMissingRemoteHostname(t *testing.T) {
	entry := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			Hostname: "api.example.com",
		},
	}

	manifest, err := serverManifestFromCatalogEntryManifest(false, true, entry, types.MCPServerManifest{})
	require.NoError(t, err)
	require.NotNil(t, manifest.RemoteConfig)
	assert.Equal(t, "api.example.com", manifest.RemoteConfig.Hostname)
	assert.Empty(t, manifest.RemoteConfig.URL)
}

func TestServerManifestFromCatalogEntryManifestPreservesRemoteURLTemplateConfig(t *testing.T) {
	const template = "https://${WORKSPACE}.example.com/mcp/${SPACE_ID}"
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: template,
				},
			},
		},
	}
	addExtractedEnvVarsToCatalogEntry(&entry)

	manifest, err := serverManifestFromCatalogEntryManifest(false, true, entry.Spec.Manifest, types.MCPServerManifest{})
	require.NoError(t, err)
	require.NotNil(t, manifest.RemoteConfig)
	assert.True(t, manifest.RemoteConfig.IsTemplate)
	assert.Equal(t, template, manifest.RemoteConfig.URLTemplate)
	assert.Empty(t, manifest.RemoteConfig.URL)
	assert.ElementsMatch(t, []types.MCPConfig{
		{
			Name:        "WORKSPACE",
			Key:         "WORKSPACE",
			Description: "Automatically detected variable",
			Required:    true,
			Usage:       types.Header,
		},
		{
			Name:        "SPACE_ID",
			Key:         "SPACE_ID",
			Description: "Automatically detected variable",
			Required:    true,
			Usage:       types.Header,
		},
	}, manifest.Config)
}

func TestEntryMissingAdminConfig(t *testing.T) {
	const ns = "obot-ns"

	newClient := func(t *testing.T, objects ...kclient.Object) kclient.Client {
		t.Helper()
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}
	secret := func(name string, data map[string][]byte) *corev1.Secret {
		return &corev1.Secret{Data: data, Name: name, Namespace: ns, Labels: map[string]string{"label": ""}}
	}

	tests := []struct {
		name            string
		manifest        types.MCPServerCatalogEntryManifest
		oauthConfigured bool
		client          kclient.Client
		wantFields      []string
		wantOAuth       bool
	}{
		{
			name: "required env resolved binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{
					{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						Usage:         types.Env,
					},
				},
			},
			client: newClient(t, secret("s", map[string][]byte{"k": []byte("v")})),
		},
		{
			name: "required env missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{
					{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						Usage:         types.Env,
					},
				},
			},
			client:     newClient(t),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "non-required env missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{
					{
						Key:           "TOKEN",
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						Usage:         types.Env,
					},
				},
			},
			client:     newClient(t),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "required env empty binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{
					{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						Usage:         types.Env,
					},
				},
			},
			client:     newClient(t, secret("s", map[string][]byte{"k": []byte("")})),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "required header missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL: "https://example.com",
				},
				Config: []types.MCPConfig{
					{
						Key:           "X-Api-Key",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						Usage:         types.Header,
					},
				},
			},
			client:     newClient(t),
			wantFields: []string{"header X-Api-Key"},
		},
		{
			name: "static oauth missing",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL:            "https://example.com",
					StaticOAuthRequired: true,
				},
			},
			wantOAuth: true,
		},
		{
			name: "static oauth configured",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL:            "https://example.com",
					StaticOAuthRequired: true,
				},
			},
			oauthConfigured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := v1.MCPServerCatalogEntry{
				Spec:   v1.MCPServerCatalogEntrySpec{Manifest: tt.manifest},
				Status: v1.MCPServerCatalogEntryStatus{OAuthCredentialConfigured: tt.oauthConfigured},
			}
			got, err := entryMissingAdminConfig(t.Context(), tt.client, ns, entry, "label")
			require.NoError(t, err)
			assert.Equal(t, tt.wantFields, got.SecretBoundFields)
			assert.Equal(t, tt.wantOAuth, got.StaticOAuth)

			err = got.err("entry")
			if len(tt.wantFields) == 0 && !tt.wantOAuth {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			errHTTP, ok := err.(*types.ErrHTTP)
			require.True(t, ok)
			assert.Equal(t, http.StatusBadRequest, errHTTP.Code)
			assert.Contains(t, errHTTP.Message, "catalog entry entry cannot be connected")
		})
	}
}
