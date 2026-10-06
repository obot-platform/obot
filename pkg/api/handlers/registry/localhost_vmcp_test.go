package registry

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestRegistryVMCPRequiresLocalhostCallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		enabled  bool
		instance bool
		legacy   bool
	}{
		{
			name:    "shared localhost",
			enabled: true,
		},
		{
			name: "shared HTTP",
		},
		{
			name:     "dedicated localhost",
			enabled:  true,
			instance: true,
		},
		{
			name:     "dedicated HTTP",
			instance: true,
		},
		{
			name:     "retained localhost snapshot",
			enabled:  true,
			instance: true,
			legacy:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{ObjectMeta: registryObjectMeta("vmcp1test")}
			component := types.VMCPComponent{ID: "component-1"}
			component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
			component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{LocalhostCallbackEnabled: tc.enabled && !tc.legacy}
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{{ID: "ordinary"}, component}
			vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{{
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}}
			instance := &v1.VMCPInstance{ObjectMeta: registryObjectMeta("vmcpi1test")}
			instance.Spec.Manifest.VMCPID = vmcp.Name
			if tc.legacy {
				legacy := *component.DeepCopy()
				legacy.SourceDigest = utils.Digest([]any{component, ""})
				legacy.CatalogEntry.Manifest.RemoteConfig.LocalhostCallbackEnabled = true
				instance.Spec.LegacyComponents = []types.VMCPComponent{legacy}
			}
			server := registryOrdinaryServer("ms1aggregate", v1.MCPServerSpec{
				MCPCatalogID: system.DefaultCatalog,
				VMCPID:       vmcp.Name,
			})
			server.Spec.Manifest.Runtime = types.RuntimeVMCP
			if tc.instance {
				server.Spec.VMCPID = ""
				server.Spec.VMCPInstanceID = instance.Name
			}
			h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server, vmcp, instance))
			// Both listing paths and direct lookup must use component-aware conversion.
			authenticated, err := h.collectAccessibleServers(ctx, "com.example.obot")
			require.NoError(t, err)
			anonymous, err := h.collectAccessibleServersNoAuth(ctx, "com.example.obot")
			require.NoError(t, err)
			direct, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
			require.NoError(t, err)
			require.Len(t, authenticated, 1)
			require.Len(t, anonymous, 1)
			for _, result := range []types.RegistryServerResponse{authenticated[0], anonymous[0], direct} {
				if tc.enabled {
					require.Empty(t, result.Server.Remotes)
					require.NotNil(t, result.Meta.Obot)
					require.True(t, result.Meta.Obot.ConfigurationRequired)
					require.Contains(t, result.Meta.Obot.ConfigurationMessage, "Obot CLI")
				} else {
					require.Len(t, result.Server.Remotes, 1)
					require.Equal(t, "streamable-http", result.Server.Remotes[0].Type)
				}
			}
		})
	}
}

func TestRegistryVMCPMissingComponentsNotAdvertised(t *testing.T) {
	server := registryOrdinaryServer("ms1aggregate", v1.MCPServerSpec{
		MCPCatalogID: system.DefaultCatalog,
		VMCPID:       "vmcp1missing",
	})
	server.Spec.Manifest.Runtime = types.RuntimeVMCP
	h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server))
	results, err := h.collectAccessibleServersNoAuth(ctx, "com.example.obot")
	require.NoError(t, err)
	require.Empty(t, results)
	_, err = h.findMCPServer(ctx, server.Name, "com.example.obot")
	require.ErrorContains(t, err, "resolve registry vMCP")
}

func TestRegistryVMCPProfileLocalhostCallback(t *testing.T) {
	for _, tc := range []struct {
		name          string
		group         string
		personal      bool
		wantsCallback bool
	}{
		{
			name:          "disabled localhost component",
			group:         "http-only",
			wantsCallback: false,
		},
		{
			name:          "group enables localhost component",
			group:         "localhost",
			wantsCallback: true,
		},
		{
			name:          "personal VMCP ignores profiles",
			personal:      true,
			wantsCallback: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{ObjectMeta: registryObjectMeta("vmcp1profiles")}
			if tc.personal {
				vmcp.Spec.UserID = "user-1"
			}
			component := types.VMCPComponent{ID: "localhost"}
			component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{LocalhostCallbackEnabled: true}
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{{ID: "http"}, component}
			vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{
				{
					Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{
						AllowedComponents: map[string]types.VMCPComponentSet{"http": {}},
					},
				},
				{
					Subjects: []types.Subject{{Type: types.SubjectTypeObotGroup, ID: "localhost"}},
					Permissions: types.VMCPProfilePermissions{
						AllowedComponents: map[string]types.VMCPComponentSet{"localhost": {}},
					},
				},
			}
			server := registryOrdinaryServer("ms1profiles", v1.MCPServerSpec{
				MCPCatalogID: system.DefaultCatalog,
				VMCPID:       vmcp.Name,
			})
			server.Spec.Manifest.Runtime = types.RuntimeVMCP
			h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server, vmcp))
			ctx.User = &kuser.DefaultInfo{
				UID:   "user-1",
				Extra: map[string][]string{"obot_groups": {tc.group}},
			}
			listed, err := h.collectAccessibleServers(ctx, "com.example.obot")
			require.NoError(t, err)
			require.Len(t, listed, 1)
			direct, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
			require.NoError(t, err)
			for _, result := range []types.RegistryServerResponse{listed[0], direct} {
				if tc.wantsCallback {
					require.Empty(t, result.Server.Remotes)
					require.Contains(t, result.Meta.Obot.ConfigurationMessage, "Obot CLI")
				} else {
					require.Len(t, result.Server.Remotes, 1)
				}
			}
			anonymous, err := h.collectAccessibleServersNoAuth(ctx, "com.example.obot")
			require.NoError(t, err)
			require.Len(t, anonymous, 1)
			require.Empty(t, anonymous[0].Server.Remotes)
		})
	}
}
