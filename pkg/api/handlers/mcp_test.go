package handlers

import (
	"bytes"
	"context"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConvertMCPResources(t *testing.T) {
	resources := &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{CPU: "250m", Memory: "512Mi"},
		Limits:   types.MCPResourceRequests{CPU: "1", Memory: "1Gi"},
	}

	entry := ConvertMCPServerCatalogEntry(v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "entry"},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:           "entry",
				Resources:      resources,
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeRemote,
			},
		},
	}, "https://example.com")
	assert.Equal(t, resources, entry.Manifest.Resources)
	assert.Equal(t, "https://example.com/mcp-connect/entry", entry.ConnectURL)

	compositeEntry := ConvertMCPServerCatalogEntry(v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "composite-entry"},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:           "composite-entry",
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{CatalogEntryID: "component"},
					},
				},
			},
		},
	}, "https://example.com")
	assert.Equal(t, "https://example.com/mcp-connect/composite-entry", compositeEntry.ConnectURL)

	server := ConvertMCPServer(v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server"},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:      "server",
				Resources: resources,
			},
		},
	}, nil, "", "")
	assert.Equal(t, resources, server.MCPServerManifest.Resources)
}

func TestHideMultiUserCatalogEntry(t *testing.T) {
	multiUserEntry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeMultiUser,
			},
		},
	}
	singleUserEntry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
			},
		},
	}

	tests := []struct {
		name  string
		user  kuser.Info
		entry v1.MCPServerCatalogEntry
		want  bool
	}{
		{
			name:  "basic user cannot see multi-user catalog entries",
			user:  &kuser.DefaultInfo{Groups: types.RoleBasic.Groups()},
			entry: multiUserEntry,
			want:  true,
		},
		{
			name:  "basic user can see single-user entries",
			user:  &kuser.DefaultInfo{Groups: types.RoleBasic.Groups()},
			entry: singleUserEntry,
			want:  false,
		},
		{
			name:  "admin can see multi-user catalog entries",
			user:  &kuser.DefaultInfo{Groups: types.RoleAdmin.Groups()},
			entry: multiUserEntry,
			want:  false,
		},
		{
			name:  "power user plus can see multi-user catalog entries",
			user:  &kuser.DefaultInfo{Groups: types.RolePowerUserPlus.Groups()},
			entry: multiUserEntry,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HideMultiUserCatalogEntry(api.Context{User: tt.user}, tt.entry)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUpdateServerAliasUnscopedSharedServer(t *testing.T) {
	server := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "server",
			Namespace: system.DefaultNamespace,
		},
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
	require.NoError(t, storage.Get(context.Background(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "server"}, &updated))
	assert.Empty(t, updated.Spec.Alias)
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
			ObjectMeta: metav1.ObjectMeta{Name: "entry"},
			Spec: v1.MCPServerCatalogEntrySpec{
				PowerUserWorkspaceID: workspaceID,
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    "entry",
					Runtime: types.RuntimeNPX,
				},
			},
		}
	}

	baseServer := func(userID string) v1.MCPServer {
		return v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "server"},
			Spec: v1.MCPServerSpec{
				UserID:                    userID,
				MCPServerCatalogEntryName: "entry",
				Manifest: types.MCPServerManifest{
					Name:    "server",
					Runtime: types.RuntimeNPX,
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
			expected:    "https://api.example.com/api//data",
			expectError: false,
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
			result, err := applyURLTemplate(tt.template, tt.envVars)

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

func TestApplyURLTemplateEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		envVars     map[string]string
		description string
		expected    string
	}{
		{
			name:        "unmatched variable remains",
			template:    "https://${API_HOST}/api/${MISSING_VAR}/data",
			envVars:     map[string]string{"API_HOST": "api.example.com"},
			description: "Variables not in envVars should remain unchanged in the result",
			expected:    "https://api.example.com/api/${MISSING_VAR}/data",
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
			result, err := applyURLTemplate(tt.template, tt.envVars)

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
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("VAR_%d", i)
		value := fmt.Sprintf("value_%d", i)
		largeEnvVars[key] = value
	}

	template := "https://example.com/api"
	for i := 0; i < 100; i++ {
		template += fmt.Sprintf("/${VAR_%d}", i)
	}

	start := time.Now()
	result, err := applyURLTemplate(template, largeEnvVars)
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
			result, err := applyURLTemplate(tt.template, tt.envVars)

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
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "ENV_BOUND", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"}}},
		},
		RemoteConfig: &types.RemoteRuntimeConfig{
			Headers: []types.MCPHeader{
				{Key: "HEADER_BOUND", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "header"}},
			},
		},
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
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "API_TOKEN",
			SecretBinding: catalogBinding,
		}}},
		RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{
			Key:           "Authorization",
			SecretBinding: catalogBinding,
		}}},
	}
	updated := existing
	source := &types.MCPServerCatalogEntryManifest{
		Runtime:        types.RuntimeRemote,
		Env:            []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "API_TOKEN"}}},
		RemoteConfig:   &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Key: "Authorization"}}},
		ServerUserType: types.ServerUserTypeMultiUser,
	}

	markAdminAddedSecretBindings(&updated, source)

	assert.Equal(t, &types.MCPSecretBinding{Name: "catalog-secret", Key: "token", AdminAdded: true}, updated.Env[0].SecretBinding)
	require.NotNil(t, updated.RemoteConfig)
	assert.Equal(t, &types.MCPSecretBinding{Name: "catalog-secret", Key: "token", AdminAdded: true}, updated.RemoteConfig.Headers[0].SecretBinding)
}

func TestMarkAdminAddedSecretBindingsRecordsOnlyAdminOwnedBindings(t *testing.T) {
	sourceBinding := &types.MCPSecretBinding{Name: "source-secret", Key: "token"}
	adminBinding := &types.MCPSecretBinding{Name: "admin-secret", Key: "token"}
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "PINNED_ENV", SecretBinding: sourceBinding}},
			{MCPHeader: types.MCPHeader{Key: "ADMIN_ENV", SecretBinding: adminBinding}},
		},
		RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{
			{Key: "Pinned-Header", SecretBinding: sourceBinding},
			{Key: "Admin-Header", SecretBinding: adminBinding},
		}},
	}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "PINNED_ENV", SecretBinding: sourceBinding}},
		},
		RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{
			{Key: "Pinned-Header", SecretBinding: sourceBinding},
		}},
	}

	markAdminAddedSecretBindings(&manifest, source)

	assert.False(t, manifest.Env[0].SecretBinding.AdminAdded)
	assert.True(t, manifest.Env[1].SecretBinding.AdminAdded)
	require.NotNil(t, manifest.RemoteConfig)
	assert.False(t, manifest.RemoteConfig.Headers[0].SecretBinding.AdminAdded)
	assert.True(t, manifest.RemoteConfig.Headers[1].SecretBinding.AdminAdded)
}

func TestMarkAdminAddedSecretBindingsClearsClientSuppliedMetadata(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "PINNED_ENV",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token", AdminAdded: true},
		}}},
	}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "PINNED_ENV",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
		}}},
	}

	markAdminAddedSecretBindings(&manifest, source)

	assert.False(t, manifest.Env[0].SecretBinding.AdminAdded)
}

// TestServerFromMultiUserTemplateMarksAdminAddedSecretBinding reproduces the bug where a
// multi-user server deployed from a template with an admin-selected secret binding (one the
// template does not define) is not marked AdminAdded, so drift/diff treats it as catalog drift.
func TestServerFromMultiUserTemplateMarksAdminAddedSecretBinding(t *testing.T) {
	entry := types.MCPServerCatalogEntryManifest{
		Name:           "tmpl",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "GREETING", Name: "GREETING"}}},
	}
	// Admin selects a secret binding for GREETING at deploy time; the template has none.
	input := types.MCPServerManifest{
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "GREETING",
			SecretBinding: &types.MCPSecretBinding{Name: "test-secret-11", Key: "key1"},
		}}},
	}

	manifest, err := serverManifestFromCatalogEntryManifest(false, false, entry, input)
	require.NoError(t, err)
	manifest = applySecretBindingOverlay(manifest, input)
	markAdminAddedSecretBindings(&manifest, &entry)

	require.Len(t, manifest.Env, 1)
	require.NotNil(t, manifest.Env[0].SecretBinding)
	assert.True(t, manifest.Env[0].SecretBinding.AdminAdded,
		"a secret binding selected at deploy time (absent from the multi-user template) must be marked AdminAdded")
}

func TestRejectCatalogSecretBindingOverrides(t *testing.T) {
	sourceBinding := &types.MCPSecretBinding{Name: "source-secret", Key: "token"}
	source := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "PINNED_ENV",
			SecretBinding: sourceBinding,
		}}},
		RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{
			Key:           "Pinned-Header",
			SecretBinding: sourceBinding,
		}}},
	}

	t.Run("allows matching catalog binding", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Key:           "PINNED_ENV",
				SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token", AdminAdded: true},
			}}},
			RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{
				Key:           "Pinned-Header",
				SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
			}}},
		}

		require.Nil(t, rejectCatalogSecretBindingOverrides(manifest, source))
	})

	t.Run("rejects env override", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Key:           "PINNED_ENV",
				SecretBinding: &types.MCPSecretBinding{Name: "admin-secret", Key: "token"},
			}}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `env "PINNED_ENV": cannot override catalog entry secretBinding`)
	})

	t.Run("rejects env binding clear", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Key: "PINNED_ENV",
			}}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `env "PINNED_ENV": cannot override catalog entry secretBinding`)
	})

	t.Run("rejects header override", func(t *testing.T) {
		manifest := types.MCPServerManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{Headers: []types.MCPHeader{{
				Key:           "Pinned-Header",
				SecretBinding: &types.MCPSecretBinding{Name: "admin-secret", Key: "token"},
			}}},
		}

		err := rejectCatalogSecretBindingOverrides(manifest, source)
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.Code)
		assert.Contains(t, err.Message, `header "Pinned-Header": cannot override catalog entry secretBinding`)
	})
}

func TestApplySecretBindingOverlayOnlyMatchesExistingFields(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "allowed-secret", Key: "token"}
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "API_TOKEN", Value: "manual"}},
		},
		RemoteConfig: &types.RemoteRuntimeConfig{
			URL: "https://example.com/mcp",
			Headers: []types.MCPHeader{
				{Key: "Authorization", Value: "manual"},
			},
		},
	}
	overlay := types.MCPServerManifest{
		Env: []types.MCPEnv{
			{MCPHeader: types.MCPHeader{Key: "API_TOKEN", SecretBinding: binding}},
			{MCPHeader: types.MCPHeader{Key: "IGNORED", SecretBinding: binding}},
		},
		RemoteConfig: &types.RemoteRuntimeConfig{
			Headers: []types.MCPHeader{
				{Key: "Authorization", SecretBinding: binding},
				{Key: "Ignored-Header", SecretBinding: binding},
			},
		},
	}

	result := applySecretBindingOverlay(manifest, overlay)

	assert.Equal(t, binding, result.Env[0].SecretBinding)
	assert.Empty(t, result.Env[0].Value)
	require.NotNil(t, result.RemoteConfig)
	assert.Equal(t, binding, result.RemoteConfig.Headers[0].SecretBinding)
	assert.Empty(t, result.RemoteConfig.Headers[0].Value)
	assert.Len(t, result.Env, 1)
	assert.Len(t, result.RemoteConfig.Headers, 1)
}

func TestCreateServerWorkspaceSecretBindingRejected(t *testing.T) {
	handler := newCreateServerSecretBindingTestHandler()
	storage := newFakeStorage(t, &v1.PowerUserWorkspace{ObjectMeta: metav1.ObjectMeta{Name: "workspace-1", Namespace: system.DefaultNamespace}})
	localK8sClient := newCreateServerSecretBindingK8sClient(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source-secret",
			Namespace: system.DefaultNamespace,
			Labels:    map[string]string{testSecretBindingAllowedLabel: "true"},
		},
		Data: map[string][]byte{"token": []byte("secret-token")},
	})

	err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, localK8sClient, "", "workspace-1", types.MCPServer{
		MCPServerManifest: newCreateServerSecretBindingManifest("source-secret", "token"),
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `validation failed: env "API_TOKEN": secretBinding is only allowed on git-synced catalog entries, multi-user catalog entries, or admin-managed multi-user servers`)
}

func TestCreateServerRejectsMultiUserHeaderSecretBinding(t *testing.T) {
	handler := newCreateServerSecretBindingTestHandler()
	storage := newFakeStorage(t, &v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "catalog-1", Namespace: system.DefaultNamespace}})
	manifest := types.MCPServerManifest{
		Name:    "multi-user-server",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{
			Key:           "X-API-Key",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
		}}},
	}

	err := handler.CreateServer(newCreateServerSecretBindingRequest(t, storage, nil, "catalog-1", "", types.MCPServer{
		MCPServerManifest: manifest,
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretBinding is not supported for user-defined headers")
}

func TestCreateCatalogEntryRejectsMultiUserHeaderSecretBinding(t *testing.T) {
	storage := newFakeStorage(t, &v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "catalog-1", Namespace: system.DefaultNamespace}})
	manifest := types.MCPServerCatalogEntryManifest{
		Name:           "multi-user-entry",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:latest",
			Port:  8080,
			Path:  "/mcp",
		},
		MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{
			Key:           "X-API-Key",
			SecretBinding: &types.MCPSecretBinding{Name: "source-secret", Key: "token"},
		}}},
	}
	body, err := json.Marshal(manifest)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/mcp-catalogs/catalog-1/entries", bytes.NewReader(body))
	req.SetPathValue("catalog_id", "catalog-1")

	err = (&MCPCatalogHandler{mcpBackend: mcp.RuntimeBackendKubernetes, sessionManager: &mcp.SessionManager{}}).CreateEntry(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
		User:           testUserWithRole("admin", types.GroupAdmin),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "secretBinding is not supported for user-defined headers")
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
		Env: []types.MCPEnv{{
			MCPHeader: types.MCPHeader{
				Key:           "API_TOKEN",
				SecretBinding: &types.MCPSecretBinding{Name: secretName, Key: secretKey},
			},
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
				Env: []types.MCPEnv{
					{MCPHeader: types.MCPHeader{Key: "PARENT_BOUND", Required: true, SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "parent"}}},
					{MCPHeader: types.MCPHeader{Key: "PARENT_USER", Required: true}},
				},
				CompositeConfig: &types.CompositeRuntimeConfig{
					ComponentServers: []types.ComponentServer{{CatalogEntryID: "entry-bound"}, {CatalogEntryID: "entry-user"}},
				},
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
			Env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "BOUND_ENV", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"}}},
				{MCPHeader: types.MCPHeader{Key: "USER_ENV"}},
			},
			RemoteConfig: &types.RemoteRuntimeConfig{
				Headers: []types.MCPHeader{
					{Key: "BOUND_HEADER", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "header"}},
					{Key: "USER_HEADER"},
				},
			},
		},
	}, types.MCPServer{
		CatalogEntryID:         "entry-user",
		Configured:             false,
		MissingRequiredEnvVars: []string{"SHARED_KEY"},
		MCPServerManifest: types.MCPServerManifest{
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "SHARED_KEY"}}},
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
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "BOUND_DISABLED", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "env"}}}},
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
	assert.ElementsMatch(t, []types.MCPHeader{
		{Name: "WORKSPACE", Key: "WORKSPACE", Description: "Automatically detected variable", Required: true},
		{Name: "SPACE_ID", Key: "SPACE_ID", Description: "Automatically detected variable", Required: true},
	}, manifest.RemoteConfig.Headers)
}

func TestServerManifestFromCatalogEntryManifestAllowsMissingCompositeRemoteHostname(t *testing.T) {
	entry := types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{
				{
					CatalogEntryID: "remote",
					Manifest: types.MCPServerCatalogEntryManifest{
						Runtime: types.RuntimeRemote,
						RemoteConfig: &types.RemoteCatalogConfig{
							Hostname: "api.example.com",
						},
					},
				},
			},
		},
	}

	require.True(t, catalogEntryRequiresUserURL(entry))
	manifest, err := serverManifestFromCatalogEntryManifest(false, true, entry, types.MCPServerManifest{})
	require.NoError(t, err)
	require.NotNil(t, manifest.CompositeConfig)
	require.Len(t, manifest.CompositeConfig.ComponentServers, 1)
	component := manifest.CompositeConfig.ComponentServers[0]
	require.NotNil(t, component.Manifest.RemoteConfig)
	assert.Equal(t, "api.example.com", component.Manifest.RemoteConfig.Hostname)
	assert.Empty(t, component.Manifest.RemoteConfig.URL)
}

func TestAddExtractedEnvVarsToCatalogEntryRecursesIntoCompositeComponents(t *testing.T) {
	const template = "https://${WORKSPACE}.example.com/mcp/${SPACE_ID}"
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeComposite,
				CompositeConfig: &types.CompositeCatalogConfig{
					ComponentServers: []types.CatalogComponentServer{
						{
							CatalogEntryID: "remote",
							Manifest: types.MCPServerCatalogEntryManifest{
								Runtime: types.RuntimeRemote,
								RemoteConfig: &types.RemoteCatalogConfig{
									URLTemplate: template,
								},
							},
						},
					},
				},
			},
		},
	}

	addExtractedEnvVarsToCatalogEntry(&entry)
	headers := entry.Spec.Manifest.CompositeConfig.ComponentServers[0].Manifest.RemoteConfig.Headers
	assert.ElementsMatch(t, []types.MCPHeader{
		{Name: "WORKSPACE", Key: "WORKSPACE", Description: "Automatically detected variable", Required: true},
		{Name: "SPACE_ID", Key: "SPACE_ID", Description: "Automatically detected variable", Required: true},
	}, headers)
}

func TestSyncConnectServerRemoteConfigFromCatalogEntryURLTemplate(t *testing.T) {
	const template = "https://${WORKSPACE}.example.com/mcp/${SPACE_ID}"
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{IsTemplate: true},
			},
		},
	}
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					URLTemplate: template,
					Headers: []types.MCPHeader{
						{Name: "WORKSPACE", Key: "WORKSPACE", Required: true},
					},
				},
			},
		},
	}

	changed := syncConnectServerRemoteConfigFromCatalogEntry(&server, entry)
	assert.True(t, changed)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	assert.True(t, server.Spec.NeedsURL)
	assert.True(t, server.Spec.Manifest.RemoteConfig.IsTemplate)
	assert.Equal(t, template, server.Spec.Manifest.RemoteConfig.URLTemplate)
	assert.Equal(t, entry.Spec.Manifest.RemoteConfig.Headers, server.Spec.Manifest.RemoteConfig.Headers)
}

func TestSyncConnectServerRemoteConfigFromCatalogEntryHostname(t *testing.T) {
	server := v1.MCPServer{
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://old.example.com/mcp",
				},
			},
		},
	}
	entry := v1.MCPServerCatalogEntry{
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.example.com",
				},
			},
		},
	}

	changed := syncConnectServerRemoteConfigFromCatalogEntry(&server, entry)
	assert.True(t, changed)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	assert.True(t, server.Spec.NeedsURL)
	assert.Equal(t, "https://old.example.com/mcp", server.Spec.PreviousURL)
	assert.Empty(t, server.Spec.Manifest.RemoteConfig.URL)
	assert.Equal(t, "api.example.com", server.Spec.Manifest.RemoteConfig.Hostname)
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
		return &corev1.Secret{Data: data, ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: map[string]string{"label": ""}}}
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
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					},
				}},
			},
			client: newClient(t, secret("s", map[string][]byte{"k": []byte("v")})),
		},
		{
			name: "required env missing binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					},
				}},
			},
			client:     newClient(t),
			wantFields: []string{"env TOKEN"},
		},
		{
			name: "required env empty binding",
			manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Key:           "TOKEN",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					},
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
					Headers: []types.MCPHeader{{
						Key:           "X-Api-Key",
						Required:      true,
						SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
					}},
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
		{
			name: "composite component missing binding",
			manifest: composite(types.CatalogComponentServer{
				CatalogEntryID: "c1",
				Manifest: types.MCPServerCatalogEntryManifest{
					Runtime: types.RuntimeNPX,
					Env: []types.MCPEnv{{
						MCPHeader: types.MCPHeader{
							Key:           "TOKEN",
							Required:      true,
							SecretBinding: &types.MCPSecretBinding{Name: "s", Key: "k"},
						},
					}},
				},
			}),
			client:     newClient(t),
			wantFields: []string{"component c1 env TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := v1.MCPServerCatalogEntry{
				Spec:   v1.MCPServerCatalogEntrySpec{Manifest: tt.manifest},
				Status: v1.MCPServerCatalogEntryStatus{OAuthCredentialConfigured: tt.oauthConfigured},
			}
			got, err := entryMissingAdminConfig(context.Background(), tt.client, ns, entry, "label")
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

// composite builds a composite catalog entry manifest from the given components.
func composite(components ...types.CatalogComponentServer) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Runtime:         types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: components},
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
