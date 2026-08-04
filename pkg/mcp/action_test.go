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

func TestVersionedConnectIDReusesPinnedDeploymentWithoutSelectingStable(t *testing.T) {
	const (
		entryID = "catalog-entry"
		userID  = "admin-1"
	)
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: entryID, Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			DefaultVersion: 1,
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeRemote,
				RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "http://localhost/stable"},
			},
		},
	}
	version := &v1.MCPServerCatalogEntryVersion{
		ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entryID, 2), Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntryVersionSpec{
			MCPServerCatalogEntryName: entryID,
			Version:                   2,
			Active:                    true,
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeSingleUser,
				Runtime:        types.RuntimeRemote,
				RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "http://localhost/version-2"},
			},
		},
	}
	stable := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: system.MCPServerPrefix + "stable", Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName:    entryID,
			MCPServerCatalogEntryVersion: 1,
			UserID:                       userID,
			Manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "http://localhost/stable"},
			},
		},
	}
	storageClient := versionedActionTestClient(entry, version, stable)
	manager := SessionManager{storageClient: storageClient, remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}}

	firstID, err := manager.VersionedConnectID(t.Context(), entryID, 2, userID)
	require.NoError(t, err)
	require.NotEqual(t, stable.Name, firstID)
	version.Spec.Manifest.RemoteConfig.FixedURL = "http://localhost/version-2-updated"
	require.NoError(t, storageClient.Update(t.Context(), version))
	secondID, err := manager.VersionedConnectID(t.Context(), entryID, 2, userID)
	require.NoError(t, err)
	require.Equal(t, firstID, secondID)

	var servers v1.MCPServerList
	require.NoError(t, storageClient.List(t.Context(), &servers))
	require.Len(t, servers.Items, 2)
	var pinned *v1.MCPServer
	for i := range servers.Items {
		if servers.Items[i].Spec.PinnedCatalogEntryVersion {
			pinned = &servers.Items[i]
		}
	}
	require.NotNil(t, pinned)
	require.Equal(t, 2, pinned.Spec.MCPServerCatalogEntryVersion)
	require.Equal(t, userID, pinned.Spec.UserID)
	require.Equal(t, "http://localhost/version-2-updated", pinned.Spec.Manifest.RemoteConfig.URL)
	require.False(t, stable.Spec.PinnedCatalogEntryVersion)
}

func TestVersionedConnectIDCreatesAdminInstanceForMultiUserVersion(t *testing.T) {
	const (
		entryID = "multi-entry"
		userID  = "admin-1"
	)
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: entryID, Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerCatalogEntrySpec{MCPCatalogName: "mc-default"},
	}
	version := &v1.MCPServerCatalogEntryVersion{
		ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entryID, 4), Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntryVersionSpec{
			MCPServerCatalogEntryName: entryID,
			Version:                   4,
			Active:                    true,
			Manifest: types.MCPServerCatalogEntryManifest{
				ServerUserType: types.ServerUserTypeMultiUser,
				Runtime:        types.RuntimeRemote,
				RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "http://localhost/multi"},
			},
		},
	}
	storageClient := versionedActionTestClient(entry, version)
	manager := SessionManager{storageClient: storageClient, remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true}}

	connectID, err := manager.VersionedConnectID(t.Context(), entryID, 4, userID)
	require.NoError(t, err)
	require.True(t, system.IsMCPServerInstanceID(connectID))

	var instances v1.MCPServerInstanceList
	require.NoError(t, storageClient.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	require.Equal(t, userID, instances.Items[0].Spec.UserID)
	var server v1.MCPServer
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: instances.Items[0].Spec.MCPServerName}, &server))
	require.True(t, server.Spec.PinnedCatalogEntryVersion)
	require.Equal(t, "mc-default", server.Spec.MCPCatalogID)
}

func versionedActionTestClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName} }).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryVersion", func(obj kclient.Object) []string {
			return []string{strconv.Itoa(obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryVersion)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.pinnedCatalogEntryVersion", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.PinnedCatalogEntryVersion)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.UserID} }).
		WithIndex(&v1.MCPServer{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServer).Spec.Template)}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.CompositeName} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.MCPServerName} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.UserID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.template", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.MCPServerInstance).Spec.Template)}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.compositeName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.CompositeName} }).
		Build()
}
