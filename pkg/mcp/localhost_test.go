package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestLocalhostCallbackValidation(t *testing.T) {
	for _, value := range []string{"", "/oauth/callback", "/custom", "/"} {
		require.NoError(t, ValidateLocalhostCallbackPath(value), value)
	}
	for _, value := range []string{"callback", "//host/path", "/oauth/obot/callback", "/oauth/%6fbot/callback", "/a/../oauth/obot/callback", "/callback?query", "/callback#fragment", "/callback\\path", " /callback"} {
		require.Error(t, ValidateLocalhostCallbackPath(value), value)
	}
	// Both catalog and server validation reject the path before network validation.
	validator := RemoteValidator{}
	require.ErrorContains(t, validator.validateRemoteConfig(t.Context(), types.RemoteRuntimeConfig{LocalhostCallbackPath: "bad"}), "localhostCallbackPath")
	require.ErrorContains(t, validator.validateRemoteCatalogConfig(t.Context(), types.RemoteCatalogConfig{LocalhostCallbackPath: "bad"}), "localhostCallbackPath")
}

func TestConfigureRemoteRuntimePreservesLocalhostCallback(t *testing.T) {
	var config ServerConfig
	_, err := configureRemoteRuntime(&config, &types.RemoteRuntimeConfig{
		URL:                      "https://example.com/mcp",
		LocalhostCallbackEnabled: true,
		LocalhostCallbackPath:    "/custom",
	}, nil, nil)
	require.NoError(t, err)
	require.True(t, config.LocalhostCallbackEnabled)
	require.Equal(t, "/custom", config.LocalhostCallbackPath)
}
