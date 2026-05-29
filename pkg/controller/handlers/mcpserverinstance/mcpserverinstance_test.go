package mcpserverinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureUserCountsUpdatesParentServerAndCatalogEntry(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
	})

	server1 := newMCPServer("server-1")
	server1.Spec.MCPCatalogID = "default"
	server1.Spec.MCPServerCatalogEntryName = entry.Name
	server2 := newMCPServer("server-2")
	server2.Spec.MCPCatalogID = "default"
	server2.Spec.MCPServerCatalogEntryName = entry.Name

	server1User1 := newMCPServerInstance("server-1-user-1", server1.Name, "user1")
	server1User1Duplicate := newMCPServerInstance("server-1-user-1-duplicate", server1.Name, "user1")
	server1User2 := newMCPServerInstance("server-1-user-2", server1.Name, "user2")
	server2User1 := newMCPServerInstance("server-2-user-1", server2.Name, "user1")

	client := newFakeClient(t, entry, server1, server2, server1User1, server1User1Duplicate, server1User2, server2User1)
	err := (&Handler{}).EnsureUserCounts(router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    server1User1,
		Namespace: server1User1.Namespace,
		Name:      server1User1.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updatedServer v1.MCPServer
	require.NoError(t, client.Get(context.Background(), router.Key(server1.Namespace, server1.Name), &updatedServer))
	require.NotNil(t, updatedServer.Status.MCPServerInstanceUserCount)
	assert.Equal(t, 2, *updatedServer.Status.MCPServerInstanceUserCount)

	var updatedEntry v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(context.Background(), router.Key(entry.Namespace, entry.Name), &updatedEntry))
	assert.Equal(t, 3, updatedEntry.Status.UserCount)
}

func TestEnsureUserCountsClearsSingleUserParentServer(t *testing.T) {
	server := newMCPServer("single-user-server")
	count := 1
	server.Status.MCPServerInstanceUserCount = &count
	instance := newMCPServerInstance("instance-1", server.Name, "user1")

	client := newFakeClient(t, server, instance)
	err := (&Handler{}).EnsureUserCounts(router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    instance,
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updatedServer v1.MCPServer
	require.NoError(t, client.Get(context.Background(), router.Key(server.Namespace, server.Name), &updatedServer))
	assert.Nil(t, updatedServer.Status.MCPServerInstanceUserCount)
}

func TestEnsureUserCountsSkipsCatalogEntryForCompositeServer(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
	})
	entry.Status.UserCount = 4

	server := newMCPServer("component-server")
	server.Spec.MCPCatalogID = "default"
	server.Spec.MCPServerCatalogEntryName = entry.Name
	server.Spec.CompositeName = "composite-server"
	instance := newMCPServerInstance("instance-1", server.Name, "user1")

	client := newFakeClient(t, entry, server, instance)
	err := (&Handler{}).EnsureUserCounts(router.Request{
		Client:    client,
		Ctx:       context.Background(),
		Object:    instance,
		Namespace: instance.Namespace,
		Name:      instance.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updatedEntry v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(context.Background(), router.Key(entry.Namespace, entry.Name), &updatedEntry))
	assert.Equal(t, 4, updatedEntry.Status.UserCount)
}

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()

	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}, &v1.MCPServerCatalogEntry{}).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{server.Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string {
			instance := obj.(*v1.MCPServerInstance)
			if instance.Spec.MCPServerName == "" {
				return nil
			}
			return []string{instance.Spec.MCPServerName}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPServerCatalogEntry(name string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServerCatalogEntry",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: manifest,
		},
	}
}

func newMCPServer(name string) *v1.MCPServer {
	return &v1.MCPServer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
}

func newMCPServerInstance(name, serverName, userID string) *v1.MCPServerInstance {
	return &v1.MCPServerInstance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServerInstance",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.MCPServerInstanceSpec{
			MCPServerName: serverName,
			UserID:        userID,
		},
	}
}
