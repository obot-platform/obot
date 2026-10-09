package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalhostCallbackCatalogRoundTrip(t *testing.T) {
	entry := MCPServerCatalogEntryManifest{
		Runtime: RuntimeRemote,
		RemoteConfig: &RemoteCatalogConfig{
			FixedURL:                 "https://provider.example/mcp",
			LocalhostCallbackEnabled: true,
			LocalhostCallbackPath:    "/custom/callback",
		},
	}
	server, err := MapCatalogEntryToServer(entry, "", false)
	require.NoError(t, err)
	require.True(t, server.RemoteConfig.LocalhostCallbackEnabled)
	require.Equal(t, "/custom/callback", server.RemoteConfig.LocalhostCallbackPath)
	converted := server.ConvertToCatalogEntry()
	require.Equal(t, entry.RemoteConfig, converted.RemoteConfig)
}
