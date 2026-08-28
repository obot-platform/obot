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
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	kuser "k8s.io/apiserver/pkg/authentication/user"
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

func TestUpdateServerAliasUnscopedSharedServer(t *testing.T) {
	server := v1.MCPServer{
		Name:      "server",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "catalog-a",
			Manifest: types.MCPServerManifest{
				Name:    "server",
				Runtime: types.RuntimeNPX,
			},
		},
	}

	req := httptest.NewRequest(http.MethodPut, "/api/mcp-servers/server/alias", strings.NewReader(`{"alias":"new alias"}`))
	req.SetPathValue("mcp_server_id", "server")
	storage := newFakeStorage(t, &server)

	err := (&MCPHandler{}).UpdateServerAlias(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
	})
	require.Error(t, err)
	assert.True(t, types.IsNotFound(err), "expected not found error, got %v", err)

	var updated v1.MCPServer
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "server"}, &updated))
	assert.Empty(t, updated.Spec.Alias)
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

func TestTriggerUpdateScope(t *testing.T) {
	type triggerUpdateScopeTestCase struct {
		name            string
		user            kuser.Info
		server          v1.MCPServer
		entry           *v1.MCPServerCatalogEntry
		catalogID       string
		workspaceID     string
		wantShutdown    bool
		wantErrContains string
	}

	baseEntry := func(workspaceID string) *v1.MCPServerCatalogEntry {
		return &v1.MCPServerCatalogEntry{
			Name: "entry",
			Spec: v1.MCPServerCatalogEntrySpec{
				PowerUserWorkspaceID: workspaceID,
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:      "entry",
					Runtime:   types.RuntimeNPX,
					NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
				},
			},
		}
	}

	baseServer := func(userID string) v1.MCPServer {
		return v1.MCPServer{
			Name: "server",
			Spec: v1.MCPServerSpec{
				UserID:                    userID,
				MCPServerCatalogEntryName: "entry",
				Manifest: types.MCPServerManifest{
					Name:      "server",
					Runtime:   types.RuntimeNPX,
					NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
				},
			},
			Status: v1.MCPServerStatus{NeedsUpdate: true},
		}
	}
	multiUserWorkspaceServer := func(workspaceID string) v1.MCPServer {
		server := baseServer("")
		server.Spec.PowerUserWorkspaceID = workspaceID
		return server
	}
	multiUserCatalogServer := func(catalogID string) v1.MCPServer {
		server := baseServer("")
		server.Spec.MCPCatalogID = catalogID
		return server
	}
	catalogEntry := func(catalogID string) *v1.MCPServerCatalogEntry {
		entry := baseEntry("")
		entry.Spec.MCPCatalogName = catalogID
		return entry
	}

	runTriggerUpdateScopeCases := func(t *testing.T, tests []triggerUpdateScopeTestCase) {
		t.Helper()
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := tt.server
				server.Namespace = system.DefaultNamespace
				objects := []kclient.Object{&server}
				if tt.entry != nil {
					entry := *tt.entry
					entry.Namespace = system.DefaultNamespace
					objects = append(objects, &entry)
				}

				req := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/server/trigger-update", nil)
				req.SetPathValue("mcp_server_id", "server")
				if tt.catalogID != "" {
					req.SetPathValue("catalog_id", tt.catalogID)
				}
				if tt.workspaceID != "" {
					req.SetPathValue("workspace_id", tt.workspaceID)
				}

				var shutdownServerNames []string
				err := (&MCPHandler{
					shutdownMCPServer: func(serverName string) error {
						shutdownServerNames = append(shutdownServerNames, serverName)
						return nil
					},
				}).TriggerUpdate(api.Context{
					ResponseWriter: httptest.NewRecorder(),
					Request:        req,
					Storage:        newFakeStorage(t, objects...),
					User:           tt.user,
				})

				if tt.wantErrContains != "" {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.wantErrContains)
					return
				}
				require.NoError(t, err)
				if tt.wantShutdown {
					assert.Equal(t, []string{"server"}, shutdownServerNames)
				} else {
					assert.Empty(t, shutdownServerNames)
				}
			})
		}
	}

	t.Run("single-user legacy behavior", func(t *testing.T) {
		runTriggerUpdateScopeCases(t, []triggerUpdateScopeTestCase{
			{
				name:         "admin can update unowned server",
				user:         testUserWithRole("admin", types.GroupAdmin),
				server:       baseServer("owner"),
				entry:        baseEntry(""),
				wantShutdown: true,
			},
			{
				name:         "owner can update own server",
				user:         testUser("owner"),
				server:       baseServer("owner"),
				entry:        baseEntry(""),
				wantShutdown: true,
			},
			{
				name:         "catalog path is not checked for owner update",
				user:         testUser("owner"),
				server:       baseServer("owner"),
				entry:        baseEntry(""),
				catalogID:    "different-catalog",
				wantShutdown: true,
			},
			{
				name:         "non-owner can update through matching workspace",
				user:         testUser("collaborator"),
				server:       baseServer("owner"),
				entry:        baseEntry("workspace"),
				workspaceID:  "workspace",
				wantShutdown: true,
			},
			{
				name:            "non-owner without workspace is hidden",
				user:            testUser("collaborator"),
				server:          baseServer("owner"),
				entry:           baseEntry("workspace"),
				wantErrContains: "MCP server server not found",
			},
			{
				name:            "non-owner with wrong workspace is hidden",
				user:            testUser("collaborator"),
				server:          baseServer("owner"),
				entry:           baseEntry("workspace"),
				workspaceID:     "other-workspace",
				wantErrContains: "MCP server server not found",
			},
			{
				name: "component server is rejected",
				user: testUserWithRole("admin", types.GroupAdmin),
				server: func() v1.MCPServer {
					server := baseServer("owner")
					server.Spec.CompositeName = "composite"
					return server
				}(),
				entry:           baseEntry(""),
				wantErrContains: "cannot trigger update on a component server",
			},
			{
				name: "server without catalog entry is a no-op",
				user: testUser("owner"),
				server: func() v1.MCPServer {
					server := baseServer("owner")
					server.Spec.MCPServerCatalogEntryName = ""
					return server
				}(),
			},
			{
				name: "server not needing update is a no-op",
				user: testUser("owner"),
				server: func() v1.MCPServer {
					server := baseServer("owner")
					server.Status.NeedsUpdate = false
					return server
				}(),
				entry: baseEntry(""),
			},
		})
	})

	t.Run("multi-user catalog entries", func(t *testing.T) {
		runTriggerUpdateScopeCases(t, []triggerUpdateScopeTestCase{
			{
				name: "admin can update catalog multi-user server through unscoped route",
				user: &kuser.DefaultInfo{
					Name:   "admin",
					UID:    "admin",
					Groups: types.RoleAdmin.Groups(),
				},
				server:       multiUserCatalogServer("catalog-a"),
				entry:        catalogEntry("catalog-a"),
				wantShutdown: true,
			},
			{
				name: "admin cannot use catalog route for server from another catalog",
				user: &kuser.DefaultInfo{
					Name:   "admin",
					UID:    "admin",
					Groups: types.RoleAdmin.Groups(),
				},
				server:          multiUserCatalogServer("catalog-a"),
				catalogID:       "catalog-b",
				wantErrContains: "MCP server server not found",
			},
			{
				name: "admin cannot trigger update for multi-user server without catalog entry",
				user: &kuser.DefaultInfo{
					Name:   "admin",
					UID:    "admin",
					Groups: types.RoleAdmin.Groups(),
				},
				server: func() v1.MCPServer {
					server := multiUserWorkspaceServer("workspace-a")
					server.Spec.MCPServerCatalogEntryName = ""
					return server
				}(),
				workspaceID:     "workspace-a",
				wantErrContains: "cannot trigger update for a multi-user MCP server without a catalog entry",
			},
			{
				name: "power user plus can update matching workspace multi-user server",
				user: &kuser.DefaultInfo{
					Name:   "power-user-plus",
					UID:    "power-user-plus",
					Groups: types.RolePowerUserPlus.Groups(),
				},
				server:       multiUserWorkspaceServer("workspace-a"),
				entry:        baseEntry("workspace-a"),
				workspaceID:  "workspace-a",
				wantShutdown: true,
			},
			{
				name: "power user plus cannot update workspace multi-user server through another workspace route",
				user: &kuser.DefaultInfo{
					Name:   "power-user-plus",
					UID:    "power-user-plus",
					Groups: types.RolePowerUserPlus.Groups(),
				},
				server:          multiUserWorkspaceServer("workspace-a"),
				entry:           baseEntry("workspace-a"),
				workspaceID:     "workspace-b",
				wantErrContains: "MCP server server not found",
			},
			{
				name: "power user plus cannot update catalog multi-user server through catalog route",
				user: &kuser.DefaultInfo{
					Name:   "power-user-plus",
					UID:    "power-user-plus",
					Groups: types.RolePowerUserPlus.Groups(),
				},
				server:          multiUserCatalogServer("catalog-a"),
				entry:           catalogEntry("catalog-a"),
				catalogID:       "catalog-a",
				wantErrContains: "MCP server server not found",
			},
			{
				name: "basic user cannot update matching workspace multi-user server",
				user: &kuser.DefaultInfo{
					Name:   "basic",
					UID:    "basic",
					Groups: types.RoleBasic.Groups(),
				},
				server:          multiUserWorkspaceServer("workspace-a"),
				entry:           baseEntry("workspace-a"),
				workspaceID:     "workspace-a",
				wantErrContains: "MCP server server not found",
			},
		})
	})
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

func TestSanitizeConfig(t *testing.T) {
	manifest := types.MCPServerManifest{

		RemoteConfig: &types.RemoteRuntimeConfig{},
		Config: []types.MCPConfig{{
			Key:           "ENV_BOUND",
			SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"},
			Usage:         types.Env,
		},

			{
				Key:           "HEADER_BOUND",
				SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "header"},
				Usage:         types.Header,
			}},
	}

	config := map[string]string{
		"KEEP":         "value",
		"EMPTY":        "",
		"ENV_BOUND":    "should-remove",
		"HEADER_BOUND": "should-remove",
	}

	sanitizeConfig(config, manifest)

	assert.Equal(t, map[string]string{"KEEP": "value"}, config)
}

func TestMarkAdminAddedSecretBindingsDerivesOwnershipFromCurrentCatalog(t *testing.T) {
	catalogBinding := &types.MCPSecretBinding{Name: "catalog-secret", Key: "token"}
	existing := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,

		RemoteConfig: &types.RemoteRuntimeConfig{},
		Config: []types.MCPConfig{{
			Key:           "API_TOKEN",
			SecretBinding: catalogBinding,
			Usage:         types.Env,
		},
			{
				Key:           "Authorization",
				SecretBinding: catalogBinding,
				Usage:         types.Header,
			}},
	}
	updated := existing
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{
			{
				Key:   "API_TOKEN",
				Usage: types.Env,
			}, {
				Key:   "Authorization",
				Usage: types.Header,
			},
		},
		RemoteConfig: &types.RemoteCatalogConfig{},
	}

	markAdminAddedSecretBindings(&updated, source)

	assert.Equal(t, &types.MCPSecretBinding{Name: "catalog-secret", Key: "token", AdminAdded: true}, updated.Config[0].SecretBinding)
	require.NotNil(t, updated.RemoteConfig)
	assert.Equal(t, &types.MCPSecretBinding{Name: "catalog-secret", Key: "token", AdminAdded: true}, updated.Config[1].SecretBinding)
}

func TestMarkAdminAddedSecretBindingsRecordsOnlyAdminOwnedBindings(t *testing.T) {
	sourceBinding := &types.MCPSecretBinding{Name: "source-secret", Key: "token"}
	adminBinding := &types.MCPSecretBinding{Name: "admin-secret", Key: "token"}
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,

		RemoteConfig: &types.RemoteRuntimeConfig{},
		Config: []types.MCPConfig{{
			Key:           "PINNED_ENV",
			SecretBinding: sourceBinding,
			Usage:         types.Env,
		},
			{
				Key:           "ADMIN_ENV",
				SecretBinding: adminBinding,
				Usage:         types.Env,
			},

			{
				Key:           "Pinned-Header",
				SecretBinding: sourceBinding,
				Usage:         types.Header,
			},
			{
				Key:           "Admin-Header",
				SecretBinding: adminBinding,
				Usage:         types.Header,
			}},
	}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{
			{
				Key:           "PINNED_ENV",
				SecretBinding: sourceBinding,
				Usage:         types.Env,
			}, {
				Key:           "Pinned-Header",
				SecretBinding: sourceBinding,
				Usage:         types.Header,
			},
		},
		RemoteConfig: &types.RemoteCatalogConfig{},
	}

	markAdminAddedSecretBindings(&manifest, source)

	assert.False(t, manifest.Config[0].SecretBinding.AdminAdded)
	assert.True(t, manifest.Config[1].SecretBinding.AdminAdded)
	require.NotNil(t, manifest.RemoteConfig)
	assert.False(t, manifest.Config[2].SecretBinding.AdminAdded)
	assert.True(t, manifest.Config[3].SecretBinding.AdminAdded)
}

func TestMarkAdminAddedSecretBindingsClearsClientSuppliedMetadata(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{{
			Key:           "PINNED_ENV",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token", AdminAdded: true},
			Usage:         types.Env,
		}},
	}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{{
			Key:           "PINNED_ENV",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
			Usage:         types.Env,
		}},
	}

	markAdminAddedSecretBindings(&manifest, source)

	assert.False(t, manifest.Config[0].SecretBinding.AdminAdded)
}

// TestServerFromMultiUserTemplateMarksAdminAddedSecretBinding reproduces the bug where a
// multi-user server deployed from a template with an admin-selected secret binding (one the
// template does not define) is not marked AdminAdded, so drift/diff treats it as catalog drift.
func TestServerFromMultiUserTemplateMarksAdminAddedSecretBinding(t *testing.T) {
	entry := types.MCPServerCatalogEntryManifest{
		Name:    "tmpl",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		Config: []types.MCPConfig{{
			Key:  "GREETING",
			Name: "GREETING",
			Usage: types.

				// Admin selects a secret binding for GREETING at deploy time; the template has none.
				Env,
		}},
	}

	input := types.MCPServerManifest{
		Config: []types.MCPConfig{{
			Key:           "GREETING",
			SecretBinding: &types.MCPSecretBinding{Name: "test-secret-11", Key: "key1"},
			Usage:         types.Env,
		}},
	}

	manifest, err := serverManifestFromCatalogEntryManifest(false, false, entry, input)
	require.NoError(t, err)
	manifest = applySecretBindingOverlay(manifest, input)
	markAdminAddedSecretBindings(&manifest, &entry)

	require.Len(t, manifest.Config, 1)
	require.NotNil(t, manifest.Config[0].SecretBinding)
	assert.True(t, manifest.Config[0].SecretBinding.AdminAdded,
		"a secret binding selected at deploy time (absent from the multi-user template) must be marked AdminAdded")
}

func TestRejectCatalogSecretBindingOverrides(t *testing.T) {
	sourceBinding := &types.MCPSecretBinding{Name: "source-secret", Key: "token"}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Config: []types.MCPConfig{
			{
				Key:           "PINNED_ENV",
				SecretBinding: sourceBinding,
				Usage:         types.Env,
			}, {
				Key:           "Pinned-Header",
				SecretBinding: sourceBinding,
				Usage:         types.Header,
			},
		},
		RemoteConfig: &types.RemoteCatalogConfig{},
	}

	t.Run("allows matching catalog binding", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,

			RemoteConfig: &types.RemoteRuntimeConfig{},
			Config: []types.MCPConfig{{
				Key:           "PINNED_ENV",
				SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token", AdminAdded: true},
				Usage:         types.Env,
			},
				{
					Key:           "Pinned-Header",
					SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
					Usage:         types.Header,
				}},
		}

		require.Nil(t, rejectCatalogSecretBindingOverrides(manifest, source, true))
	})

	t.Run("rejects env override", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			Config: []types.MCPConfig{{
				Key:           "PINNED_ENV",
				SecretBinding: &types.MCPSecretBinding{Name: "admin-secret", Key: "token"},
				Usage:         types.Env,
			}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source, true)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `env "PINNED_ENV": cannot override catalog entry secretBinding`)
	})

	t.Run("rejects env binding clear", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			Config: []types.MCPConfig{{
				Key:   "PINNED_ENV",
				Usage: types.Env,
			}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source, true)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `env "PINNED_ENV": cannot override catalog entry secretBinding`)
	})

	t.Run("rejects env binding omission for full update", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source, true)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `env "PINNED_ENV": cannot omit catalog entry secretBinding`)
	})

	t.Run("allows env binding omission for partial overlay", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
		}

		require.Nil(t, rejectCatalogSecretBindingOverrides(manifest, source, false))
	})

	t.Run("rejects header override", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,

			RemoteConfig: &types.RemoteRuntimeConfig{},
			Config: []types.MCPConfig{{
				Key:           "PINNED_ENV",
				SecretBinding: sourceBinding,
				Usage:         types.Env,
			},
				{
					Key:           "Pinned-Header",
					SecretBinding: &types.MCPSecretBinding{Name: "admin-secret", Key: "token"},
					Usage:         types.Header,
				}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source, true)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `header "Pinned-Header": cannot override catalog entry secretBinding`)
	})
}

func TestApplySecretBindingOverlayOnlyMatchesExistingFields(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "allowed-secret", Key: "token"}
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,

		RemoteConfig: &types.RemoteRuntimeConfig{
			URL: "https://example.com/mcp",
		},
		Config: []types.MCPConfig{{
			Key:   "API_TOKEN",
			Value: "manual",
			Usage: types.Env,
		},

			{
				Key:   "Authorization",
				Value: "manual",
				Usage: types.Header,
			}},
	}
	overlay := types.MCPServerManifest{

		RemoteConfig: &types.RemoteRuntimeConfig{},
		Config: []types.MCPConfig{{
			Key:           "API_TOKEN",
			SecretBinding: binding,
			Usage:         types.Env,
		},
			{
				Key:           "IGNORED",
				SecretBinding: binding,
				Usage:         types.Env,
			},

			{
				Key:           "Authorization",
				SecretBinding: binding,
				Usage:         types.Header,
			},
			{
				Key:           "Ignored-Header",
				SecretBinding: binding,
				Usage:         types.Header,
			}},
	}

	result := applySecretBindingOverlay(manifest, overlay)

	assert.Equal(t, binding, result.Config[0].SecretBinding)
	assert.Empty(t, result.Config[0].Value)
	require.NotNil(t, result.RemoteConfig)
	assert.Equal(t, binding, result.Config[1].SecretBinding)
	assert.Empty(t, result.Config[1].Value)
	assert.Len(t, result.Config, 2)
}

func TestCreateServerWorkspaceSecretBindingRejected(t *testing.T) {
	handler := newCreateServerSecretBindingTestHandler()
	storage := newFakeStorage(t, &v1.PowerUserWorkspace{Name: "workspace-1", Namespace: system.DefaultNamespace})
	localK8sClient := newCreateServerSecretBindingK8sClient(t, &corev1.Secret{
		Name:      "source-secret",
		Namespace: system.DefaultNamespace,
		Labels:    map[string]string{testSecretBindingAllowedLabel: "true"},
		Data:      map[string][]byte{"token": []byte("secret-token")},
	})

	err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, localK8sClient, "", "workspace-1", types.MCPServer{
		MCPServerManifest: newCreateServerSecretBindingManifest("source-secret", "token"),
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `validation failed: env "API_TOKEN": secretBinding is only allowed on git-synced catalog entries, multi-user catalog entries, or admin-managed multi-user servers`)
}

func TestCreateServerRejectsMissingSecretBinding(t *testing.T) {
	handler := newCreateServerSecretBindingTestHandler()
	entry := v1.MCPServerCatalogEntry{
		Name: "entry-1", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "catalog-1",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:    "multi-user-entry",
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "example/mcp:latest",
					Port:  8080,
					Path:  "/mcp",
				},
				Config: []types.MCPConfig{{
					Key:           "API_TOKEN",
					SecretBinding: &types.MCPSecretBinding{Name: "missing-secret", Key: "token"},
					Usage:         types.Env,
				}},
			},
		},
	}
	storage := newFakeStorage(t,
		&v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace},
		&entry,
	)

	err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, newCreateServerSecretBindingK8sClient(t), "catalog-1", "", types.MCPServer{
		CatalogEntryID: "entry-1",
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unavailable Kubernetes Secret")

	var servers v1.MCPServerList
	require.NoError(t, storage.List(t.Context(), &servers))
	assert.Empty(t, servers.Items)
}

func TestCreateServerRejectsMultiUserHeaderSecretBinding(t *testing.T) {
	handler := newCreateServerSecretBindingTestHandler()
	storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace})
	manifest := types.MCPServerManifest{
		Name:    "multi-user-server",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		Config: []types.MCPConfig{{
			Key:           "X-API-Key",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
			Usage:         types.Header,
			UserAllowed:   true,
		}},
	}

	err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, nil, "catalog-1", "", types.MCPServer{
		MCPServerManifest: manifest,
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretBinding is not supported for user-defined headers")
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

func newCreateServerSecretBindingTestHandler() *MCPHandler {
	return &MCPHandler{
		mcpSessionManager:         &mcp.SessionManager{},
		mcpRuntimeBackend:         mcp.RuntimeBackendKubernetes,
		secretBindingAllowedLabel: testSecretBindingAllowedLabel,
	}
}

func newCreateServerSecretBindingManifest(secretName, secretKey string) types.MCPServerManifest {
	return types.MCPServerManifest{
		Name:    "secret-bound-server",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		Config: []types.MCPConfig{{
			Key:           "API_TOKEN",
			SecretBinding: &types.MCPSecretBinding{Name: secretName, Key: secretKey},
			Usage:         types.Env,
		}},
	}
}

func newCreateServerSecretBindingRequest(t *testing.T, storageClient storage.Client, localK8sClient kclient.Client, catalogID, workspaceID string, input types.MCPServer) api.Context {
	t.Helper()

	body, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/mcpservers", bytes.NewReader(body))
	if catalogID != "" {
		req.SetPathValue("catalog_id", catalogID)
	}
	if workspaceID != "" {
		req.SetPathValue("workspace_id", workspaceID)
	}

	groups := []string(nil)
	if catalogID != "" {
		groups = append(groups, types.GroupAdmin)
	}

	return api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storageClient,
		User:           testUserWithRole("user-1", groups...),
		LocalK8sClient: localK8sClient,
		ObotNamespace:  system.DefaultNamespace,
	}
}

func newCreateServerSecretBindingK8sClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestConvertMCPServerCompositeAggregatesOnlySecretBoundMissingConfig(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,

				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{{CatalogEntryID: "entry-bound"}, {CatalogEntryID: "entry-user"}},
				},
				Config: []types.MCPConfig{{
					Key:           "PARENT_BOUND",
					Required:      true,
					SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "parent"},
					Usage:         types.Env,
				},
					{
						Key:      "PARENT_USER",
						Required: true,
						Usage:    types.Env,
					}},
			},
		},
	}

	converted := ConvertMCPServer(server, map[string]string{}, "", "", types.MCPServer{
		CatalogEntryID:         "entry-bound",
		Configured:             false,
		MissingRequiredEnvVars: []string{"BOUND_ENV", "USER_ENV"},
		MissingRequiredHeaders: []string{"BOUND_HEADER", "USER_HEADER"},
		MCPServerManifest: types.MCPServerManifest{
			Runtime: types.RuntimeRemote,

			RemoteConfig: &types.RemoteRuntimeConfig{},
			Config: []types.MCPConfig{{
				Key:           "BOUND_ENV",
				SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"},
				Usage:         types.Env,
			},
				{
					Key:   "USER_ENV",
					Usage: types.Env,
				},

				{
					Key:           "BOUND_HEADER",
					SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "header"},
					Usage:         types.Header,
				},
				{
					Key:   "USER_HEADER",
					Usage: types.Header,
				}},
		},
	}, types.MCPServer{
		CatalogEntryID:         "entry-user",
		Configured:             false,
		MissingRequiredEnvVars: []string{"SHARED_KEY"},
		MCPServerManifest: types.MCPServerManifest{
			Config: []types.MCPConfig{{
				Key:   "SHARED_KEY",
				Usage: types.Env,
			}},
		},
	})

	assert.False(t, converted.Configured)
	assert.Equal(t, []string{"PARENT_BOUND", "BOUND_ENV"}, converted.MissingRequiredEnvVars)
	assert.Equal(t, []string{"BOUND_HEADER"}, converted.MissingRequiredHeaders)
}

func TestConvertMCPServerCompositeSkipsDisabledAndConfiguredComponents(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{
						{CatalogEntryID: "entry-disabled", Disabled: true},
						{CatalogEntryID: "entry-configured"},
					},
				},
			},
		},
	}

	converted := ConvertMCPServer(server, nil, "", "", types.MCPServer{
		CatalogEntryID:         "entry-disabled",
		Configured:             false,
		MissingRequiredEnvVars: []string{"BOUND_DISABLED"},
		MCPServerManifest: types.MCPServerManifest{
			Config: []types.MCPConfig{{
				Key:           "BOUND_DISABLED",
				SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"},
				Usage:         types.Env,
			}},
		},
	}, types.MCPServer{
		CatalogEntryID: "entry-configured",
		Configured:     true,
	})

	assert.True(t, converted.Configured)
	assert.Empty(t, converted.MissingRequiredEnvVars)
	assert.Empty(t, converted.MissingRequiredHeaders)
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
				Config: []types.MCPConfig{{
					Key:           "TOKEN",
					Required:      true,
					SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					Usage:         types.Env,
				}},
			},
			client: newClient(t, secret("s", map[string][]byte{"k": []byte("v")})),
		},
		{
			name: "required env missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{
					Key:           "TOKEN",
					Required:      true,
					SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					Usage:         types.Env,
				}},
			},
			client:     newClient(t),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "non-required env missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{
					Key:           "TOKEN",
					SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					Usage:         types.Env,
				}},
			},
			client:     newClient(t),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "required env empty binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Config: []types.MCPConfig{{
					Key:           "TOKEN",
					Required:      true,
					SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					Usage:         types.Env,
				}},
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
				Config: []types.MCPConfig{{
					Key:           "X-Api-Key",
					Required:      true,
					SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					Usage:         types.Header,
				}},
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

func TestUpdateServerFromCatalogEntryCopiesResources(t *testing.T) {
	resources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{CPU: "500m", Memory: "512Mi"},
		Limits:   types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
	}
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    "server",
				Runtime: types.RuntimeContainerized,
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{CPU: "250m", Memory: "256Mi"},
				},
			},
		},
	}
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:      "entry",
				Runtime:   types.RuntimeContainerized,
				Resources: resources,
			},
		},
	}

	updateServerFromCatalogEntry(&server, entry)
	assert.Equal(t, resources, server.Spec.Manifest.Resources)
}

func TestUpdateServerFromCatalogEntryCopiesTunnelName(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://old.example.com/mcp"},
			},
		},
	}
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL:   "https://api.example.com/mcp",
					TunnelName: "mcptunnel-office",
				},
			},
		},
	}

	updateServerFromCatalogEntry(&server, entry)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	assert.Equal(t, "https://api.example.com/mcp", server.Spec.Manifest.RemoteConfig.URL)
	assert.Equal(t, "mcptunnel-office", server.Spec.Manifest.RemoteConfig.TunnelName)
}

func TestUpdateServerFromCatalogEntryPreservesValidHostnameURL(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://api.example.com/mcp",
				},
			},
		},
	}
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname:   "api.example.com",
					TunnelName: "mcptunnel-office",
				},
			},
		},
	}

	updateServerFromCatalogEntry(&server, entry)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	assert.Equal(t, "https://api.example.com/mcp", server.Spec.Manifest.RemoteConfig.URL)
	assert.Equal(t, "mcptunnel-office", server.Spec.Manifest.RemoteConfig.TunnelName)
	assert.False(t, server.Spec.NeedsURL)
	assert.Empty(t, server.Spec.PreviousURL)
}
