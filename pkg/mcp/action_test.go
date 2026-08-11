package mcp

import (
	"strconv"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestServerInstanceHeadersRejectsUnknownOption(t *testing.T) {
	instance := v1.MCPServerInstance{Spec: v1.MCPServerInstanceSpec{MultiUserConfig: &types.MultiUserConfig{
		UserDefinedHeaders: []types.MCPHeader{{
			Key:      "REGION",
			Required: true,
			Options:  []types.MCPConfigurationOption{{Value: "us", Name: "United States"}},
		}},
	}}}

	names, values, missing := serverInstanceHeaders(instance, map[string]string{"REGION": "eu"})
	require.Empty(t, names)
	require.Empty(t, values)
	require.Equal(t, []string{"REGION"}, missing)
}

func TestAddExtractedEnvVarsDefaultsToSensitive(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPServerManifest
	}{
		{
			name: "npx argument remains sensitive",
			manifest: types.MCPServerManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Args: []string{"--token=${TOKEN}"}},
			},
		},
		{
			name: "undeclared remote URL variable defaults to sensitive",
			manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://${HOST}/mcp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := v1.MCPServer{Spec: v1.MCPServerSpec{Manifest: tt.manifest}}
			addExtractedEnvVars(&server)
			require.Len(t, server.Spec.Manifest.Env, 1)
			require.True(t, server.Spec.Manifest.Env[0].Sensitive)
		})
	}
}

func TestAddExtractedEnvVarsToCatalogEntryManifestRemoteFields(t *testing.T) {
	t.Run("missing variable becomes env", func(t *testing.T) {
		manifest := types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${HOST}/mcp"},
		}
		addExtractedEnvVarsToCatalogEntryManifest(&manifest)
		require.Len(t, manifest.Env, 1)
		require.Equal(t, "HOST", manifest.Env[0].Key)
		require.True(t, manifest.Env[0].Required)
		require.False(t, manifest.Env[0].Sensitive)
	})

	t.Run("explicit env is preserved", func(t *testing.T) {
		expected := types.MCPEnv{MCPHeader: types.MCPHeader{Key: "HOST", Name: "Host", Required: true, Sensitive: true}}
		manifest := types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			Env:          []types.MCPEnv{expected},
			RemoteConfig: &types.RemoteCatalogConfig{URLTemplate: "https://${HOST}/mcp"},
		}
		addExtractedEnvVarsToCatalogEntryManifest(&manifest)
		require.Equal(t, []types.MCPEnv{expected}, manifest.Env)
	})

	t.Run("legacy header does not create duplicate env", func(t *testing.T) {
		manifest := types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{
				URLTemplate: "https://${HOST}/mcp",
				Headers:     []types.MCPHeader{{Key: "HOST", Required: true}},
			},
		}
		addExtractedEnvVarsToCatalogEntryManifest(&manifest)
		require.Empty(t, manifest.Env)
		require.Len(t, manifest.RemoteConfig.Headers, 1)
	})
}

func TestServerOrInstanceFromConnectURLCreatesRemoteServerThatNeedsUserURL(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      entryID,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{
					Hostname: "api.example.com",
				},
			},
		},
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(entry).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		Build()

	manager := SessionManager{storageClient: storageClient}
	server, instance, err := manager.serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.NoError(t, err)
	require.Empty(t, instance.Name)
	require.NotEmpty(t, server.Name)
	require.Equal(t, entryID, server.Spec.MCPServerCatalogEntryName)
	require.Equal(t, userID, server.Spec.UserID)
	require.True(t, server.Spec.NeedsURL)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	require.Equal(t, "api.example.com", server.Spec.Manifest.RemoteConfig.Hostname)
	require.Empty(t, server.Spec.Manifest.RemoteConfig.URL)
}

func TestServerOrInstanceFromConnectURLRejectsResourcesAbovePersistedMaximum(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	maximum := resource.MustParse("500m")
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: entryID, Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeNPX,
				NPXConfig:      &types.NPXRuntimeConfig{Package: "example"},
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{CPU: "1"},
				},
			},
		},
	}
	settings := &v1.K8sSettings{
		ObjectMeta: metav1.ObjectMeta{Name: system.K8sSettingsName, Namespace: system.DefaultNamespace},
		Spec:       v1.K8sSettingsSpec{MaxCPURequest: &maximum},
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(entry, settings).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		Build()

	manager := SessionManager{
		runtimeBackend: RuntimeBackendKubernetes,
		storageClient:  storageClient,
	}
	server, instance, err := manager.serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.ErrorContains(t, err, "resources.requests.cpu 1 exceeds configured maximum 500m")
	require.Empty(t, server.Name)
	require.Empty(t, instance.Name)

	var servers v1.MCPServerList
	require.NoError(t, storageClient.List(t.Context(), &servers))
	require.Empty(t, servers.Items)
}
