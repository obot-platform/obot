package vmcpinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSharedComponentConnectionLifecycle(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1shared",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{{
					ID: "one",
					CatalogEntry: types.MCPServerCatalogEntrySnapshot{
						Manifest: types.MCPServerCatalogEntryManifest{
							Name:    "one",
							Runtime: types.RuntimeRemote,
							RemoteConfig: &types.RemoteCatalogConfig{
								FixedURL: "https://example.com/mcp",
							},
						},
					},
				}},
				Profiles: allowAllProfiles(),
			},
		},
	}
	first := &v1.VMCPInstance{
		Name:      "vmcpi1first",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			UserID: "7",
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
		},
	}
	second := first.DeepCopy()
	second.Name = "vmcpi1second"
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, first, second).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }), first.Spec.UserID).Build()
	handler := &Handler{userInfo: func(_ context.Context, id uint) (kuser.Info, error) {
		require.Equal(t, uint(7), id)
		return &kuser.DefaultInfo{UID: "7"}, nil
	}}
	reconcile := func() {
		t.Helper()
		require.NoError(t, vmcphandler.EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil))
		for _, instance := range []*v1.VMCPInstance{first, second} {
			req := router.Request{Ctx: t.Context(), Client: client, Object: instance}
			require.NoError(t, handler.EnsureMCPServers(req, nil))
			require.NoError(t, handler.EnsureMCPServerInstances(req, nil))
		}
	}
	for _, single := range []bool{false, true, false} {
		vmcp.Spec.Manifest.Components[0].ForceSingleUser = single
		require.NoError(t, client.Update(t.Context(), vmcp))
		reconcile()
		reconcile()
		var servers v1.MCPServerList
		var connections v1.MCPServerInstanceList
		require.NoError(t, client.List(t.Context(), &servers))
		require.NoError(t, client.List(t.Context(), &connections))
		if single {
			require.Len(t, servers.Items, 2)
			for i := range connections.Items {
				require.False(t, connections.Items[i].DeletionTimestamp.IsZero())
				// Simulate credential finalization before the next sharing transition.
				connections.Items[i].Finalizers = nil
				require.NoError(t, client.Update(t.Context(), &connections.Items[i]))
			}
		} else {
			require.Len(t, servers.Items, 1)
			require.Len(t, connections.Items, 2)
			require.NotEqual(t, connections.Items[0].Spec.VMCPInstanceID, connections.Items[1].Spec.VMCPInstanceID)
			for _, connection := range connections.Items {
				require.Equal(t, servers.Items[0].Name, connection.Spec.MCPServerName)
				require.Equal(t, "7", connection.Spec.UserID)
				require.Contains(t, connection.Finalizers, v1.MCPServerInstanceFinalizer)
				require.Contains(t, connection.DeleteRefs(), v1.Ref{ObjType: &v1.VMCPInstance{}, Name: connection.Spec.VMCPInstanceID})
			}
		}
	}

	// Profile loss affects dedicated servers, while shared backing servers and connections remain.
	var serversBefore v1.MCPServerList
	var connectionsBefore v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &serversBefore))
	require.NoError(t, client.List(t.Context(), &connectionsBefore))
	vmcp.Spec.Manifest.Profiles[0].Permissions.AllowAllComponents = false
	require.NoError(t, client.Update(t.Context(), vmcp))
	reconcile()
	reconcile()
	var serversAfter v1.MCPServerList
	var connectionsAfter v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &serversAfter))
	require.NoError(t, client.List(t.Context(), &connectionsAfter))
	require.Equal(t, serversBefore.Items, serversAfter.Items)
	require.Equal(t, connectionsBefore.Items, connectionsAfter.Items)

	vmcp.Spec.Manifest.Components = nil
	require.NoError(t, client.Update(t.Context(), vmcp))
	reconcile()
	var connections v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &connections))
	for _, connection := range connections.Items {
		require.False(t, connection.DeletionTimestamp.IsZero())
	}
}

// A component the user leaves out keeps the server or connection it already has, so its
// configuration and OAuth survive, but one it never had isn't created.
func TestLeftOutComponentKeepsItsServers(t *testing.T) {
	remote := func(id string, single bool) types.VMCPComponent {
		return types.VMCPComponent{
			ID:              id,
			ForceSingleUser: single,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Name:         id,
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/" + id},
			}},
		}
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1choose",
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			UserSelectableComponents: true,
			Components:               []types.VMCPComponent{remote("shared", false), remote("dedicated", true)},
			Profiles:                 allowAllProfiles(),
		}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1choose",
		Namespace: "default",
		Spec:      v1.VMCPInstanceSpec{UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	fresh := instance.DeepCopy()
	fresh.Name = "vmcpi1fresh"
	fresh.Spec.Manifest.ComponentSet = map[string]types.VMCPComponentSet{"shared": {}}
	client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, instance, fresh).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }), instance.Spec.UserID).Build()
	handler := &Handler{userInfo: func(context.Context, uint) (kuser.Info, error) { return &kuser.DefaultInfo{UID: "7"}, nil }}
	reconcile := func(choice map[string]types.VMCPComponentSet) {
		t.Helper()
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
		instance.Spec.Manifest.ComponentSet = choice
		require.NoError(t, client.Update(t.Context(), instance))
		require.NoError(t, vmcphandler.EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil))
		for _, i := range []*v1.VMCPInstance{instance, fresh} {
			req := router.Request{Ctx: t.Context(), Client: client, Object: i}
			require.NoError(t, handler.EnsureMCPServers(req, nil))
			require.NoError(t, handler.EnsureMCPServerInstances(req, nil))
		}
	}
	owned := func(name string) (dedicated, connections int) {
		t.Helper()
		var servers v1.MCPServerList
		var conns v1.MCPServerInstanceList
		require.NoError(t, client.List(t.Context(), &servers, kclient.MatchingFields{"spec.vmcpInstanceID": name}))
		require.NoError(t, client.List(t.Context(), &conns, kclient.MatchingFields{"spec.vmcpInstanceID": name}))
		return len(servers.Items), len(conns.Items)
	}

	reconcile(nil)
	dedicated, connections := owned(instance.Name)
	require.Equal(t, [2]int{1, 1}, [2]int{dedicated, connections})

	// Leaving out either component keeps what it has.
	for _, choice := range []string{"shared", "dedicated"} {
		reconcile(map[string]types.VMCPComponentSet{choice: {}})
		dedicated, connections = owned(instance.Name)
		require.Equal(t, [2]int{1, 1}, [2]int{dedicated, connections}, "chose only %s", choice)
	}

	// A connection that never chose the dedicated component has no server for it.
	dedicated, connections = owned(fresh.Name)
	require.Equal(t, [2]int{0, 1}, [2]int{dedicated, connections})

	// Removing a component from the vMCP still removes what it had.
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
	vmcp.Spec.Manifest.Components = vmcp.Spec.Manifest.Components[:1]
	require.NoError(t, client.Update(t.Context(), vmcp))
	reconcile(map[string]types.VMCPComponentSet{"shared": {}})
	dedicated, _ = owned(instance.Name)
	require.Zero(t, dedicated)
}
