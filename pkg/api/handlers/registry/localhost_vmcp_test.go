package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type failingRegistryInstanceStorage struct {
	kclient.WithWatch
}

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

func TestRegistryCanonicalVMCPUsesUserInstance(t *testing.T) {
	for _, tc := range []struct {
		name            string
		currentCallback bool
		legacyCallback  bool
		legacyDisabled  bool
		staleSnapshot   bool
		noInstance      bool
		profileDisabled bool
		wantCallback    bool
	}{
		{
			name:           "retained snapshot requires CLI",
			legacyCallback: true,
			wantCallback:   true,
		},
		{
			name:            "retained snapshot supports HTTP",
			currentCallback: true,
		},
		{
			name:            "legacy disabled component does not require CLI",
			currentCallback: true,
			legacyCallback:  true,
			legacyDisabled:  true,
		},
		{
			name:           "upgraded component discards retained callback",
			legacyCallback: true,
			staleSnapshot:  true,
		},
		{
			name:            "profile-disabled retained component requires no callback",
			currentCallback: true,
			legacyCallback:  true,
			profileDisabled: true,
		},
		{
			name:            "first connection uses current callback",
			currentCallback: true,
			noInstance:      true,
			wantCallback:    true,
		},
		{
			name:           "first connection uses current HTTP configuration",
			legacyCallback: true,
			noInstance:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{ObjectMeta: registryObjectMeta("vmcp1canonical")}
			component := types.VMCPComponent{ID: "remote"}
			component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{LocalhostCallbackEnabled: tc.currentCallback}
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{component}
			vmcp.Spec.Manifest.Profiles = []types.VMCPProfile{{
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}}
			if tc.profileDisabled {
				vmcp.Spec.Manifest.Profiles = nil
			}
			legacy := *component.DeepCopy()
			legacy.SourceDigest = utils.Digest([]any{component, ""})
			legacy.CatalogEntry.Manifest.RemoteConfig.LocalhostCallbackEnabled = tc.legacyCallback
			oldest := &v1.VMCPInstance{ObjectMeta: registryObjectMeta("vmcpi1oldest")}
			oldest.CreationTimestamp = metav1.NewTime(time.Unix(300, 0))
			legacyCreatedAt := metav1.NewTime(time.Unix(100, 0))
			oldest.Spec.LegacyCreatedAt = &legacyCreatedAt
			oldest.Spec.UserID = "user-1"
			oldest.Spec.Manifest.VMCPID = vmcp.Name
			oldest.Spec.LegacyComponents = []types.VMCPComponent{legacy}
			if tc.staleSnapshot {
				vmcp.Spec.ComponentStaticConfigurationHashes = map[string]string{component.ID: "upgraded"}
			}
			if tc.legacyDisabled {
				oldest.Spec.LegacyDisabledComponents = []string{component.ID}
			}
			newer := &v1.VMCPInstance{ObjectMeta: registryObjectMeta("vmcpi1newer")}
			newer.CreationTimestamp = metav1.NewTime(time.Unix(200, 0))
			newer.Spec.UserID = "user-1"
			newer.Spec.Manifest.VMCPID = vmcp.Name
			otherUser := newer.DeepCopy()
			otherUser.Name = "vmcpi1other"
			otherUser.CreationTimestamp = metav1.NewTime(time.Unix(50, 0))
			otherUser.Spec.UserID = "user-2"

			server := registryOrdinaryServer("ms1canonical", v1.MCPServerSpec{
				MCPCatalogID: system.DefaultCatalog,
				VMCPID:       vmcp.Name,
			})
			server.Spec.Manifest.Runtime = types.RuntimeVMCP
			second := server.DeepCopy()
			second.Name = "ms1second"
			objects := []kclient.Object{server, second, vmcp, otherUser}
			if !tc.noInstance {
				objects = append(objects, oldest, newer)
			}
			storage := newRegistryTestStorage(objects...)
			h, ctx := registryTestHandlerAndContext(t, storage)
			listed, err := h.collectAccessibleServers(ctx, "com.example.obot")
			require.NoError(t, err)
			require.Len(t, listed, 2)
			require.Equal(t, 1, storage.instanceLists, "registry listing must load instances once")
			direct, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
			require.NoError(t, err)
			for _, result := range append(listed, direct) {
				if tc.wantCallback {
					require.Empty(t, result.Server.Remotes)
					require.NotNil(t, result.Meta.Obot)
					require.Contains(t, result.Meta.Obot.ConfigurationMessage, "Obot CLI")
				} else {
					require.Len(t, result.Server.Remotes, 1)
				}
			}
			// Anonymous reads must not inspect a particular user's private snapshots.
			anonymous, err := h.collectAccessibleServersNoAuth(ctx, "com.example.obot")
			require.NoError(t, err)
			require.Len(t, anonymous, 2)
			ctx.User = &kuser.DefaultInfo{UID: "anonymous", Groups: []string{authz.UnauthenticatedGroup}}
			anonymousDirect, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
			require.NoError(t, err)
			for _, result := range append(anonymous, anonymousDirect) {
				require.Equal(t, tc.currentCallback, len(result.Server.Remotes) == 0)
			}
			require.Equal(t, 2, storage.instanceLists, "anonymous paths must not query user instances")
			if tc.noInstance {
				return
			}
			ctx.User = &kuser.DefaultInfo{UID: "user-1"}
			// Explicit instance URLs must keep selecting that instance, not the oldest.
			server.Spec.VMCPInstanceID = newer.Name
			explicit, err := serverRequiresLocalhostCallback(ctx.Context(), storage, *server, ctx.User, nil)
			require.NoError(t, err)
			require.Equal(t, tc.currentCallback && !tc.profileDisabled, explicit)
		})
	}
}

func (s failingRegistryInstanceStorage) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if _, ok := list.(*v1.VMCPInstanceList); ok {
		return fmt.Errorf("instance lookup unavailable")
	}
	return s.WithWatch.List(ctx, list, opts...)
}

func TestRegistryCanonicalVMCPInstanceLookupFailure(t *testing.T) {
	server := registryOrdinaryServer("ms1canonical", v1.MCPServerSpec{
		MCPCatalogID: system.DefaultCatalog,
		VMCPID:       "vmcp1canonical",
	})
	server.Spec.Manifest.Runtime = types.RuntimeVMCP
	h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server))
	ctx.Storage = failingRegistryInstanceStorage{WithWatch: ctx.Storage}
	listed, err := h.collectAccessibleServers(ctx, "com.example.obot")
	require.ErrorContains(t, err, "instance lookup unavailable")
	require.Empty(t, listed)
	direct, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
	require.ErrorContains(t, err, "instance lookup unavailable")
	require.Empty(t, direct.Server.Remotes)
}
