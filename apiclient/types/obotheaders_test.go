package types

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
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
	data, err := json.Marshal(entry)
	require.NoError(t, err)
	var restored MCPServerCatalogEntryManifest
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, entry.InjectObotHeaders, restored.InjectObotHeaders)
}

func TestObotHeaderValidationDoesNotMutateSharedManifest(t *testing.T) {
	manifest := MCPServerManifest{
		Runtime:           RuntimeRemote,
		InjectObotHeaders: []ObotHeader{"x-obot-user-id", "x-obot-idp-groups"},
	}
	original := slices.Clone(manifest.InjectObotHeaders)
	var workers sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		workers.Go(func() {
			for range 100 {
				if err := manifest.ValidateConfig(); err != nil {
					failures <- err
					return
				}
			}
		})
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Equal(t, original, manifest.InjectObotHeaders)
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

func TestObotHeaderDecodingPreservesSelections(t *testing.T) {
	var manifest MCPServerManifest
	err := json.Unmarshal([]byte(`{"runtime":"remote","injectObotHeaders":["x-obot-user-id","x-obot-idp-groups"]}`), &manifest)
	require.NoError(t, err)
	require.Equal(t, []ObotHeader{"x-obot-user-id", "x-obot-idp-groups"}, manifest.InjectObotHeaders)
	require.NoError(t, manifest.ValidateConfig())
}
