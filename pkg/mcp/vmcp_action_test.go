package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestServerForActionWithConnectIDResolvesVMCPWithoutWrapper(t *testing.T) {
	const (
		vmcpID = "vmcp1action"
		userID = "user-1"
	)

	instanceID := "vmcpi1-action-instance"
	componentID := "component"
	vmcp := &v1.VMCP{
		Name:      vmcpID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName: "Action VMCP",
				Profiles: []types.VMCPProfile{{
					Subjects:    []types.Subject{{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				}},
				Components: []types.VMCPComponent{
					{
						ID:              componentID,
						Name:            "component",
						ForceSingleUser: true,
						CatalogEntry: types.MCPServerCatalogEntrySnapshot{
							Manifest: types.MCPServerCatalogEntryManifest{
								Name:    "Component",
								Runtime: types.RuntimeRemote,
							},
						},
					},
				},
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      instanceID,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcpID,
			},
			UserID: userID,
		},
	}
	componentServer := &v1.MCPServer{
		Name:      "ms1-action-component",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    "Component",
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://component.example.test/mcp",
				},
			},
			UserID:          userID,
			VMCPInstanceID:  instanceID,
			VMCPComponentID: componentID,
		},
	}

	storageClient := newVMCPTestStorage(vmcp, instance, componentServer)
	manager := &SessionManager{
		httpListenPort: vmcpTestListenPort,
		storageClient:  storageClient,
	}

	user := &kuser.DefaultInfo{UID: userID, Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}}
	gotID, gotServer, gotConfig, err := manager.ServerForActionWithConnectID(t.Context(), vmcpID, user)
	require.NoError(t, err)
	require.Equal(t, vmcpID, gotID)
	require.Equal(t, vmcpID, gotServer.Name)
	require.Equal(t, types.RuntimeVMCP, gotConfig.Runtime)
	require.Equal(t, instanceID, gotConfig.MCPServerName)
	require.Equal(t, vmcp.Spec.Manifest.DisplayName, gotConfig.MCPServerDisplayName)
	require.Len(t, gotConfig.Components, 1)
	require.Equal(t, componentServer.Name, gotConfig.Components[0].Name)
	require.False(t, gotConfig.Components[0].DisableTools)

	_, _, secondConfig, err := manager.ServerForActionWithConnectID(t.Context(), vmcpID, user)
	require.NoError(t, err)
	require.Equal(t, gotConfig, secondConfig)
	_, actionConfig, err := manager.ServerForAction(t.Context(), vmcpID, user)
	require.NoError(t, err)
	require.Equal(t, gotConfig, actionConfig)
	connectConfig, err := manager.ServerConfigForVMCP(t.Context(), vmcpID, user)
	require.NoError(t, err)
	require.Equal(t, gotConfig, connectConfig)

	var instances v1.VMCPInstanceList
	require.NoError(t, storageClient.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	require.Equal(t, instanceID, instances.Items[0].Name)

	var servers v1.MCPServerList
	require.NoError(t, storageClient.List(t.Context(), &servers))
	for _, server := range servers.Items {
		require.NotEqual(t, vmcpID, server.Name, "resolving a vMCP must not create a wrapper MCPServer")
	}
}
