package mcp

import (
	"strconv"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddExtractedEnvVarsToCatalogEntryManifestPreservesRemoteHeaders(t *testing.T) {
	manifest := &types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			URLTemplate: "https://${EXISTING}.example.com/${DETECTED}",
			Headers: []types.MCPHeader{{
				Name: "Existing", Key: "EXISTING", Required: true,
			}},
		},
	}

	AddExtractedEnvVarsToCatalogEntryManifest(manifest)

	require.Empty(t, manifest.Env)
	require.ElementsMatch(t, []types.MCPHeader{
		{Name: "Existing", Key: "EXISTING", Required: true},
		{Name: "DETECTED", Key: "DETECTED", Description: "Automatically detected variable", Required: true},
	}, manifest.RemoteConfig.Headers)
}

func TestServerOrInstanceFromConnectURLCreatesRemoteServerThatNeedsUserURL(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	entry := &v1.MCPServerCatalogEntry{
		Name:      entryID,
		Namespace: system.DefaultNamespace,
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

func TestServerOrInstanceFromConnectURLCopiesStaticCatalogConfiguration(t *testing.T) {
	const (
		entryID     = "catalog-entry"
		catalogID   = "default"
		userID      = "user-1"
		staticKey   = "SECRET_KEY"
		staticValue = "catalog-secret"
	)

	entry := &v1.MCPServerCatalogEntry{
		Name:      entryID,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: catalogID,
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeNPX,
				NPXConfig:      &types.NPXRuntimeConfig{Package: "example"},
				Env: []types.MCPEnv{{
					MCPHeader: types.MCPHeader{
						Name:            "Static Secret",
						Key:             staticKey,
						Sensitive:       true,
						ValueConfigured: true,
					},
				}},
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
	gatewayClient := newActionTestGatewayClient(t)
	require.NoError(t, StoreStaticCredentialSecrets(
		t.Context(), gatewayClient, CatalogEntryStaticCredentialContext(entryID), entryID,
		map[string]string{staticKey: staticValue},
	))

	manager := SessionManager{storageClient: storageClient, gatewayClient: gatewayClient}
	server, instance, err := manager.serverOrInstanceFromConnectURL(t.Context(), entryID, userID)
	require.NoError(t, err)
	require.Empty(t, instance.Name)
	require.True(t, server.Spec.IsSingleUser())
	require.Empty(t, server.Spec.MCPCatalogID)
	require.Empty(t, server.Spec.PowerUserWorkspaceID)
	require.Empty(t, server.Spec.Manifest.Env[0].Value)
	require.True(t, server.Spec.Manifest.Env[0].ValueConfigured)

	credentialContext := serverCredentialContext(serverScope(server), server.Name)
	staticSecrets, err := StaticCredentialSecrets(t.Context(), gatewayClient, credentialContext, server.Name)
	require.NoError(t, err)
	require.Equal(t, staticValue, staticSecrets[staticKey])
}

func newActionTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	client := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestServerOrInstanceFromConnectURLRejectsResourcesAbovePersistedMaximum(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "user-1"
	)

	maximum := resource.MustParse("500m")
	entry := &v1.MCPServerCatalogEntry{
		Name: entryID, Namespace: system.DefaultNamespace,
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
		Name: system.K8sSettingsName, Namespace: system.DefaultNamespace,
		Spec: v1.K8sSettingsSpec{MaxCPURequest: &maximum},
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
