package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestVMCPRemoteUserURL(t *testing.T) {
	manifest, err := types.MapCatalogEntryToServer(types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			Hostname: "github.example.com",
		},
	}, "", true)
	require.NoError(t, err)
	server := v1.MCPServer{Spec: v1.MCPServerSpec{
		Manifest:        manifest,
		VMCPInstanceID:  "vmcpi1test",
		VMCPComponentID: "github",
	}}
	config, missing, err := ServerToServerConfig(server, nil, "user", "scope", "", nil)
	require.NoError(t, err)
	require.Contains(t, missing, "__url")
	require.Empty(t, config.URL)

	config, missing, err = ServerToServerConfig(server, nil, "user", "scope", "", map[string]string{"__url": "https://github.example.com/mcp"})
	require.NoError(t, err)
	require.Empty(t, missing)
	require.Equal(t, "https://github.example.com/mcp", config.URL)

	_, _, err = ServerToServerConfig(server, nil, "user", "scope", "", map[string]string{"__url": "https://other.example.com/mcp"})
	require.ErrorContains(t, err, "does not match required hostname")
}
