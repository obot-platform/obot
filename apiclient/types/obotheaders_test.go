package types

import (
	"slices"
	"strings"
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
			name:    "remote case insensitive",
			runtime: RuntimeRemote,
			headers: []ObotHeader{"x-obot-api-key-name", "x-obot-idp-user-id", "x-obot-idp-groups"},
		},
		{
			name:    "containerized unsupported",
			runtime: RuntimeContainerized,
			headers: []ObotHeader{ObotHeaderUserID},
			wantErr: true,
		},
		{
			name:    "containerized without selections remains valid",
			runtime: RuntimeContainerized,
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
			original := slices.Clone(tt.headers)
			manifest := MCPServerManifest{Runtime: tt.runtime, InjectObotHeaders: tt.headers}
			err := manifest.ValidateConfig()
			require.Equal(t, original, manifest.InjectObotHeaders)
			if tt.wantErr {
				require.Error(t, err)
				require.Error(t, manifest.ConvertToCatalogEntry().ValidateConfig())
				return
			}
			require.NoError(t, err)
			for _, header := range manifest.InjectObotHeaders {
				canonical, ok := CanonicalObotHeader(string(header))
				require.True(t, ok)
				require.NotEmpty(t, canonical)
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

}

func TestReservedObotHeaderConfiguration(t *testing.T) {
	for _, header := range ObotHeaders() {
		t.Run(string(header), func(t *testing.T) {
			manifest := MCPServerManifest{
				Runtime: RuntimeRemote,
				Config: []MCPConfig{{
					Key:   strings.ToLower(string(header)),
					Usage: Header,
					Value: "spoofed",
				}},
			}
			require.ErrorContains(t, manifest.ValidateConfig(), "reserved for injectObotHeaders")
			require.ErrorContains(t, manifest.ConvertToCatalogEntry().ValidateConfig(), "reserved for injectObotHeaders")
			manifest.Config[0].Usage = Env
			require.NoError(t, manifest.ValidateConfig(), "only HTTP headers reserve these names")
		})
	}
}
