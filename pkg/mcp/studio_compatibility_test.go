package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestStudioCompatibilityAllowsMultiUserRemoteCatalogEntries(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{
		ServerUserType: types.ServerUserTypeMultiUser,
		Runtime:        types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://8.8.8.8/mcp",
		},
		MultiUserConfig: &types.MultiUserConfig{
			UserDefinedHeaders: []types.MCPHeader{
				{
					Name:      "API Key",
					Key:       "X-API-Key",
					Required:  true,
					Sensitive: true,
				},
			},
		},
	}

	require.NoError(t, ValidateCatalogEntryManifest(t.Context(), manifest, true, ValidationOptions{}))
}
