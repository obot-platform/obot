package mcp

import (
	"strconv"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
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
