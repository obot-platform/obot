package vmcpinstance

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSharedComponentConnectionLifecycle(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one", CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Name: "one", Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		}}}},
	}}}
	first := &v1.VMCPInstance{Name: "vmcpi1first", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	second := first.DeepCopy()
	second.Name = "vmcpi1second"
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, first, second).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).Build()
	handler := New(nil)
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
	vmcp.Spec.Manifest.Components = nil
	require.NoError(t, client.Update(t.Context(), vmcp))
	reconcile()
	var connections v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &connections))
	for _, connection := range connections.Items {
		require.False(t, connection.DeletionTimestamp.IsZero())
	}
}
