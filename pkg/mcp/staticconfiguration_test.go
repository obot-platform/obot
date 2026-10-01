package mcp

import (
	"context"
	"maps"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryCredentialStore struct {
	credentials map[[2]string]map[string]string
	upserts     int
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{credentials: make(map[[2]string]map[string]string)}
}

func (s *memoryCredentialStore) RevealCredential(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	for _, credentialContext := range contexts {
		if secrets, ok := s.credentials[[2]string{credentialContext, name}]; ok {
			return gatewaytypes.Credential{Context: credentialContext, Name: name, Secrets: maps.Clone(secrets)}, nil
		}
	}
	return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{Contexts: contexts, Name: name}
}

func (s *memoryCredentialStore) UpsertCredential(_ context.Context, credential gatewaytypes.Credential) error {
	s.upserts++
	s.credentials[[2]string{credential.Context, credential.Name}] = maps.Clone(credential.Secrets)
	return nil
}

func TestExtractStaticConfiguration(t *testing.T) {
	tests := []struct {
		name         string
		config       []types.MCPConfig
		previous     map[string]string
		keepPrevious bool
		wantValues   map[string]string
		wantStatic   []bool
		wantErr      string
	}{
		{
			name: "literal values become static",
			config: []types.MCPConfig{
				{Key: "TOKEN", Value: "secret", Usage: types.Env},
				{Key: "USER", Usage: types.Env},
			},
			wantValues: map[string]string{"TOKEN": "secret"},
			wantStatic: []bool{true, false},
		},
		{
			name: "secret bindings are never static",
			config: []types.MCPConfig{
				{Key: "TOKEN", Static: true, SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "token"}, Usage: types.Env},
			},
			wantValues: map[string]string{},
			wantStatic: []bool{false},
		},
		{
			name: "static field without a value keeps the previous value",
			config: []types.MCPConfig{
				{Key: "TOKEN", Static: true, Usage: types.Env},
			},
			previous:     map[string]string{"TOKEN": "secret"},
			keepPrevious: true,
			wantValues:   map[string]string{"TOKEN": "secret"},
			wantStatic:   []bool{true},
		},
		{
			name: "submitted value replaces the previous value",
			config: []types.MCPConfig{
				{Key: "TOKEN", Static: true, Value: "rotated", Usage: types.Env},
			},
			previous:     map[string]string{"TOKEN": "secret"},
			keepPrevious: true,
			wantValues:   map[string]string{"TOKEN": "rotated"},
			wantStatic:   []bool{true},
		},
		{
			name: "static field without a value cannot keep the previous value",
			config: []types.MCPConfig{
				{Key: "TOKEN", Static: true, Usage: types.Env},
			},
			previous: map[string]string{"TOKEN": "secret"},
			wantErr:  `static configuration "TOKEN" requires a value`,
		},
		{
			name: "static field without any value is rejected",
			config: []types.MCPConfig{
				{Key: "TOKEN", Static: true, Usage: types.Env},
			},
			keepPrevious: true,
			wantErr:      `static configuration "TOKEN" requires a value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := ExtractStaticConfiguration(tt.config, tt.previous, tt.keepPrevious)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantValues, values)
			for i, field := range tt.config {
				assert.Empty(t, field.Value, field.Key)
				assert.Equal(t, tt.wantStatic[i], field.Static, field.Key)
			}
		})
	}
}

func TestStoreStaticConfigurationRevisions(t *testing.T) {
	store := newMemoryCredentialStore()
	manifest := func(value string) *types.MCPServerCatalogEntryManifest {
		return &types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{{Key: "TOKEN", Value: value, Usage: types.Header}}}
	}

	first := manifest("secret")
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", first, ""))
	require.NotEmpty(t, first.StaticConfigurationRevision)
	assert.True(t, first.Config[0].Static)
	assert.Empty(t, first.Config[0].Value)
	values, err := RevealStaticConfiguration(t.Context(), store, "entry", first.StaticConfigurationRevision)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"TOKEN": "secret"}, values)

	unchanged := manifest("secret")
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", unchanged, first.StaticConfigurationRevision))
	assert.Equal(t, first.StaticConfigurationRevision, unchanged.StaticConfigurationRevision)
	assert.Equal(t, 1, store.upserts)

	rotated := manifest("rotated")
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", rotated, first.StaticConfigurationRevision))
	assert.NotEqual(t, first.StaticConfigurationRevision, rotated.StaticConfigurationRevision)
	// The old revision stays available to copies made from the old manifest.
	values, err = RevealStaticConfiguration(t.Context(), store, "entry", first.StaticConfigurationRevision)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"TOKEN": "secret"}, values)

	removed := manifest("")
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", removed, rotated.StaticConfigurationRevision))
	assert.Empty(t, removed.StaticConfigurationRevision)
	assert.False(t, removed.Config[0].Static)
}

func TestResolveServerStaticConfiguration(t *testing.T) {
	store := newMemoryCredentialStore()
	entryManifest := types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
		{Key: "TOKEN", Value: "secret", Usage: types.Header},
		{Key: "USER", Usage: types.Header},
	}}
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", &entryManifest, ""))

	server := v1.MCPServer{Spec: v1.MCPServerSpec{
		MCPServerCatalogEntryName: "entry",
		Manifest: types.MCPServerManifest{
			Config:                      entryManifest.Config,
			StaticConfigurationRevision: entryManifest.StaticConfigurationRevision,
		},
	}}
	resolved, err := ResolveServerStaticConfiguration(t.Context(), store, server)
	require.NoError(t, err)
	assert.Equal(t, "secret", resolved.Spec.Manifest.Config[0].Value)
	assert.Empty(t, resolved.Spec.Manifest.Config[1].Value)
	assert.Empty(t, server.Spec.Manifest.Config[0].Value, "the stored manifest must not be modified")

	// A missing credential has no values, which only launching treats as an error.
	values, err := RevealStaticConfiguration(t.Context(), store, "entry", "missing")
	require.NoError(t, err)
	assert.Empty(t, values)
	server.Spec.Manifest.StaticConfigurationRevision = "missing"
	_, err = ResolveServerStaticConfiguration(t.Context(), store, server)
	require.ErrorContains(t, err, `static configuration "TOKEN" for catalog entry "entry" is not available`)

	// An entry that has not published its revision yet has no values for its Static fields.
	server.Spec.Manifest.StaticConfigurationRevision = ""
	_, err = ResolveServerStaticConfiguration(t.Context(), store, server)
	require.ErrorContains(t, err, `static configuration "TOKEN" for catalog entry "entry" is not available`)
}

func TestServerToServerConfigUsesResolvedStaticHeader(t *testing.T) {
	store := newMemoryCredentialStore()
	entryManifest := types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
		Config:       []types.MCPConfig{{Key: "AUTHORIZATION", Value: "token", Prefix: "Bearer ", Required: true, Usage: types.Header}},
	}
	require.NoError(t, StoreStaticConfiguration(t.Context(), store, "entry", &entryManifest, ""))
	serverManifest, err := types.MapCatalogEntryToServer(entryManifest, "", false)
	require.NoError(t, err)
	require.Equal(t, entryManifest.StaticConfigurationRevision, serverManifest.StaticConfigurationRevision)

	server := v1.MCPServer{Spec: v1.MCPServerSpec{MCPServerCatalogEntryName: "entry", Manifest: serverManifest}}
	resolved, err := ResolveServerStaticConfiguration(t.Context(), store, server)
	require.NoError(t, err)

	serverConfig, missing, err := ServerToServerConfig(resolved, nil, "user", "scope", "catalog", nil)
	require.NoError(t, err)
	assert.Empty(t, missing)
	// Static values are used as configured, without the user-supplied prefix.
	assert.Equal(t, []string{"AUTHORIZATION=token"}, serverConfig.Headers)
}
