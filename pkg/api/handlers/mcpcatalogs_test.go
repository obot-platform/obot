package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeCapacityInfoProvider struct {
	serverNames []string
	info        types.MCPCapacityInfo
	err         error
}

func TestStaticOAuthCredentialSecrets(t *testing.T) {
	t.Run("requires client ID", func(t *testing.T) {
		_, err := staticOAuthCredentialSecrets(types.MCPServerOAuthCredentialRequest{ClientSecret: "secret"})
		require.ErrorContains(t, err, "clientID is required")
	})

	t.Run("accepts public client", func(t *testing.T) {
		secrets, err := staticOAuthCredentialSecrets(types.MCPServerOAuthCredentialRequest{ClientID: " public-client "})
		require.NoError(t, err)
		require.Equal(t, map[string]string{"CLIENT_ID": "public-client"}, secrets)
	})

	t.Run("accepts confidential client", func(t *testing.T) {
		secrets, err := staticOAuthCredentialSecrets(types.MCPServerOAuthCredentialRequest{
			ClientID:     " client-id ",
			ClientSecret: " client-secret ",
		})
		require.NoError(t, err)
		require.Equal(t, map[string]string{
			"CLIENT_ID":     "client-id",
			"CLIENT_SECRET": "client-secret",
		}, secrets)
	})
}

func TestAcceptCatalogEntryOwnership(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Annotations: map[string]string{
			"example.com/keep": "true",
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			Editable:  false,
			Detached:  true,
			SourceURL: "https://github.com/obot-platform/mcp-catalog",
			Manifest: types.MCPServerCatalogEntryManifest{
				EntryKey:    "context7",
				UpgradeNote: "Review configuration changes before upgrading.",
			},
		},
	}

	acceptCatalogEntryOwnership(entry)

	assert.True(t, entry.Spec.Editable)
	assert.Empty(t, entry.Spec.SourceURL)
	assert.Empty(t, entry.Spec.Manifest.EntryKey)
	assert.Empty(t, entry.Spec.Manifest.UpgradeNote)
	assert.False(t, entry.Spec.Detached)
	assert.Equal(t, "true", entry.Annotations["example.com/keep"])
}

func (f *fakeCapacityInfoProvider) GetCapacityInfoForServers(_ context.Context, serverNames []string) (types.MCPCapacityInfo, error) {
	f.serverNames = slices.Clone(serverNames)
	return f.info, f.err
}

func TestMCPCatalogHandlerGetEntryCapacity(t *testing.T) {
	tests := []struct {
		name             string
		entryCatalogName string
		entryRuntime     types.Runtime
		servers          []v1.MCPServer
		providerInfo     types.MCPCapacityInfo
		providerErr      error
		wantServerNames  []string
		wantErr          string
		wantResponse     types.MCPCapacityInfo
	}{
		{
			name:             "returns capacity for matching deployments",
			entryCatalogName: "catalog-1",
			entryRuntime:     types.RuntimeContainerized,
			servers: []v1.MCPServer{{
				Name: "server-1", Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: "entry-1"},
			}},
			providerInfo:    types.MCPCapacityInfo{Source: types.CapacitySourceDeployments, ActiveDeployments: 1},
			wantServerNames: []string{"server-1"},
			wantResponse:    types.MCPCapacityInfo{Source: types.CapacitySourceDeployments, ActiveDeployments: 1},
		},
		{
			name:             "excludes template deployments",
			entryCatalogName: "catalog-1",
			entryRuntime:     types.RuntimeContainerized,
			servers: []v1.MCPServer{
				{Name: "template-server", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: "entry-1", Template: true}},
				{Name: "server-1", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: "entry-1"}},
			},
			providerInfo:    types.MCPCapacityInfo{ActiveDeployments: 1},
			wantServerNames: []string{"server-1"},
			wantResponse:    types.MCPCapacityInfo{ActiveDeployments: 1},
		},
		{
			name:             "rejects composite catalog entry",
			entryCatalogName: "catalog-1",
			entryRuntime:     types.RuntimeComposite,
			wantErr:          "capacity is only supported for hosted catalog entries",
		},
		{
			name:             "rejects remote catalog entry",
			entryCatalogName: "catalog-1",
			entryRuntime:     types.RuntimeRemote,
			wantErr:          "capacity is only supported for hosted catalog entries",
		},
		{
			name:             "rejects entry from another catalog",
			entryCatalogName: "catalog-2",
			wantErr:          "entry does not belong to catalog",
		},
		{
			name:             "rejects unsupported backend",
			entryCatalogName: "catalog-1",
			entryRuntime:     types.RuntimeContainerized,
			providerErr:      &mcp.ErrNotSupportedByBackend{Feature: "capacity info", Backend: "docker"},
			wantErr:          "feature capacity info is not supported by docker backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeCapacityInfoProvider{info: tt.providerInfo, err: tt.providerErr}
			objects := []kclient.Object{
				&v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace},
				&v1.MCPServerCatalogEntry{
					Name: "entry-1", Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						MCPCatalogName: tt.entryCatalogName,
						Manifest:       types.MCPServerCatalogEntryManifest{Runtime: tt.entryRuntime},
					},
				},
			}
			for i := range tt.servers {
				server := tt.servers[i]
				objects = append(objects, &server)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/mcp-catalogs/catalog-1/entries/entry-1/mcp-capacity", nil)
			req.SetPathValue("catalog_id", "catalog-1")
			req.SetPathValue("entry_id", "entry-1")
			rec := httptest.NewRecorder()
			err := (&MCPCatalogHandler{capacityInfoProvider: provider}).GetEntryCapacity(api.Context{
				ResponseWriter: rec,
				Request:        req,
				Storage:        newFakeStorage(t, objects...),
			})

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, provider.serverNames)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantServerNames, provider.serverNames)

			var got types.MCPCapacityInfo
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
			assert.Equal(t, tt.wantResponse, got)
		})
	}
}

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

func TestPrepareTempServerConfigDoesNotUseBoundSecretInURL(t *testing.T) {
	const (
		namespace = "obot-ns"
		label     = "allowed-secret"
		key       = "WORKSPACE"
	)
	localK8sClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&corev1.Secret{
		Name: "remote-secret", Namespace: namespace, Labels: map[string]string{label: "true"},
		Data: map[string][]byte{"token": []byte("secret-value")},
	}).Build()
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{
			Key: key, Required: true}},
		RemoteConfig: &types.RemoteRuntimeConfig{
			IsTemplate:  true,
			URLTemplate: "https://example.com/mcp/${WORKSPACE}",
			Headers: []types.MCPHeader{{
				Key: key, SecretBinding: &types.MCPSecretBinding{Name: "remote-secret", Key: "token"},
			}},
		},
	}
	input := map[string]string{key: "user-value"}
	options := mcp.ValidationOptions{RemoteMCPURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
		AllowLocalhostMCP: true,
		AllowPrivateIPMCP: true,
		AllowLinkLocalMCP: true,
	}}

	merged, err := prepareTempServerConfig(t.Context(), localK8sClient, namespace, label, &manifest, input, false, options)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/mcp/user-value", manifest.RemoteConfig.URL)
	require.NotContains(t, manifest.RemoteConfig.URL, "secret-value")
	require.Equal(t, "secret-value", merged[key])
	require.Equal(t, "user-value", input[key])
}

func TestPrepareTempServerConfigRejectsUnknownOption(t *testing.T) {
	manifest := types.MCPServerManifest{
		Runtime: types.RuntimeRemote,
		Env: []types.MCPEnv{{
			Key: "REGION", Required: true,
			Options: []types.MCPConfigurationOption{{Name: "US", Value: "us"}}}},
		RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
	}
	_, err := prepareTempServerConfig(t.Context(), fake.NewClientBuilder().Build(), "obot-ns", "allowed", &manifest, map[string]string{"REGION": "forged"}, false, mcp.ValidationOptions{})
	require.Error(t, err)
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.Contains(t, httpErr.Message, "not one of the configured options")
}

func TestMCPCatalogHandlerUpdateEntryHydratesComponentManifestFromCatalogEntry(t *testing.T) {
	originalComponentManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Original Component",
		Runtime:        types.RuntimeNPX,
		ServerUserType: types.ServerUserTypeSingleUser,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/original"},
	}
	differentComponentManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Different Component",
		Runtime:        types.RuntimeRemote,
		ServerUserType: types.ServerUserTypeSingleUser,
		RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "https://different.example/mcp"},
	}
	updatedManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Composite",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{
				CatalogEntryID: "original-component",
				Manifest:       differentComponentManifest,
			},
		}},
	}
	body, err := json.Marshal(updatedManifest)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPut, "/api/mcp-catalogs/custom/entries/composite", bytes.NewReader(body))
	req.SetPathValue("catalog_id", "custom")
	req.SetPathValue("entry_id", "composite")
	storage := newFakeStorage(t,
		&v1.MCPCatalog{Name: "custom", Namespace: system.DefaultNamespace},
		&v1.MCPServerCatalogEntry{
			Name: "original-component", Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: "custom",
				Manifest:       originalComponentManifest,
			},
		},
		&v1.MCPServerCatalogEntry{
			Name: "different-component", Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: "custom",
				Manifest:       differentComponentManifest,
			},
		},
		&v1.MCPServerCatalogEntry{
			Name: "composite", Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: "custom",
				Editable:       true,
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:           "Composite",
					Runtime:        types.RuntimeComposite,
					ServerUserType: types.ServerUserTypeSingleUser,
					CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
						{CatalogEntryID: "original-component", Manifest: originalComponentManifest},
					}},
				},
			},
		},
	)
	err = (&MCPCatalogHandler{}).UpdateEntry(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		User:           &kuser.DefaultInfo{Groups: []string{types.GroupAdmin}},
		Storage:        storage,
	})

	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "composite",
	}, &updated))
	require.NotNil(t, updated.Spec.Manifest.CompositeConfig)
	require.Len(t, updated.Spec.Manifest.CompositeConfig.ComponentServers, 1)
	component := updated.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.NotNil(t, component.Manifest.NPXConfig)
	assert.Nil(t, component.Manifest.RemoteConfig)
}

func TestPopulateComponentManifestsHydratesMCPServerID(t *testing.T) {
	server := &v1.MCPServer{
		Name: "shared-server", Namespace: system.DefaultNamespace,
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
		Name: "component-entry", Namespace: system.DefaultNamespace,
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
			{
				CatalogEntryID: "component-entry",
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:         "Attacker Server",
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://attacker.example/mcp"},
				},
			},
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
	assert.Nil(t, component.Manifest.RemoteConfig)
}

func newPopulateComponentManifestsRequest(objects ...kclient.Object) api.Context {
	return api.Context{
		Request:        httptest.NewRequest(http.MethodGet, "/", nil),
		ResponseWriter: httptest.NewRecorder(),
		Storage: storage.Client(fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(objects...).
			Build()),
	}
}
