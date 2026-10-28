package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInjectObotHeadersValidation(t *testing.T) {
	tests := []struct {
		name    string
		runtime Runtime
		headers []ObotHeader
		wantErr bool
	}{
		{
			name:    "absent is compatible with all runtimes",
			runtime: RuntimeNPX,
		},
		{
			name:    "remote",
			runtime: RuntimeRemote,
			headers: []ObotHeader{ObotHeaderAPIKeyName, ObotHeaderUserID, ObotHeaderIDPUserID, ObotHeaderIDPGroups},
		},
		{
			name:    "containerized case insensitive",
			runtime: RuntimeContainerized,
			headers: []ObotHeader{"x-obot-api-key-name", "x-obot-idp-user-id", "x-obot-idp-groups"},
		},
		{
			name:    "unknown",
			runtime: RuntimeRemote,
			headers: []ObotHeader{"Authorization"},
			wantErr: true,
		},
		{
			name:    "duplicate ignoring case",
			runtime: RuntimeRemote,
			headers: []ObotHeader{ObotHeaderUserID, "x-obot-user-id"},
			wantErr: true,
		},
		{
			name:    "stdio unsupported",
			runtime: RuntimeNPX,
			headers: []ObotHeader{ObotHeaderUserID},
			wantErr: true,
		},
		{
			name:    "openapi unsupported",
			runtime: RuntimeOpenAPI,
			headers: []ObotHeader{ObotHeaderUserID},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := MCPServerManifest{Runtime: tt.runtime, InjectObotHeaders: tt.headers}
			err := manifest.ValidateConfig()
			if tt.wantErr {
				require.Error(t, err)
				require.Error(t, manifest.ConvertToCatalogEntry().ValidateConfig())
				return
			}
			require.NoError(t, err)
			for _, header := range manifest.InjectObotHeaders {
				canonical, ok := CanonicalObotHeader(string(header))
				require.True(t, ok)
				require.Equal(t, canonical, header)
			}
		})
	}
}

func TestInjectObotHeadersCatalogRoundTrip(t *testing.T) {
	entry := MCPServerCatalogEntryManifest{
		Runtime:           RuntimeRemote,
		RemoteConfig:      &RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		InjectObotHeaders: []ObotHeader{ObotHeaderAPIKeyName, ObotHeaderUserID, ObotHeaderIDPUserID, ObotHeaderIDPGroups},
	}
	server, err := MapCatalogEntryToServer(entry, "", false)
	require.NoError(t, err)
	require.Equal(t, entry.InjectObotHeaders, server.InjectObotHeaders)
	require.Equal(t, entry.InjectObotHeaders, server.ConvertToCatalogEntry().InjectObotHeaders)
	server.InjectObotHeaders[0] = ObotHeaderUserEmail
	require.Equal(t, ObotHeaderAPIKeyName, entry.InjectObotHeaders[0], "server must not alias catalog selection")
	cloned := entry.DeepCopy()
	cloned.InjectObotHeaders[0] = ObotHeaderUserName
	require.Equal(t, ObotHeaderAPIKeyName, entry.InjectObotHeaders[0])
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	var restored MCPServerCatalogEntryManifest
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, entry.InjectObotHeaders, restored.InjectObotHeaders)
}
