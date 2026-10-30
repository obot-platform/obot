package mcpcatalog

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestNormalizeManifestObotHeaders(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers []types.ObotHeader
		want    []types.ObotHeader
		wantErr string
	}{
		{
			name: "absent",
		},
		{
			name:    "canonical names",
			headers: []types.ObotHeader{"x-obot-user-id", "X-OBOT-IDP-GROUPS"},
			want:    []types.ObotHeader{types.ObotHeaderUserID, types.ObotHeaderIDPGroups},
		},
		{
			name:    "unknown names remain invalid",
			headers: []types.ObotHeader{"X-Unknown"},
			want:    []types.ObotHeader{"X-Unknown"},
			wantErr: "unsupported injectObotHeaders",
		},
		{
			name:    "case variant duplicates remain invalid",
			headers: []types.ObotHeader{"x-obot-user-id", "X-Obot-User-Id"},
			want:    []types.ObotHeader{types.ObotHeaderUserID, types.ObotHeaderUserID},
			wantErr: "duplicate injectObotHeaders",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entry := types.MCPServerCatalogEntryManifest{
				Runtime:           types.RuntimeRemote,
				InjectObotHeaders: tt.headers,
			}
			NormalizeManifest(&entry)
			require.Equal(t, tt.want, entry.InjectObotHeaders)
			err := entry.ValidateConfig()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateConfigurationFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  string
		field string
	}{
		{
			name:  "env",
			data:  "env: []",
			field: "env",
		},
		{
			name:  "headers",
			data:  `{"remoteConfig":{"headers":[{"name":"Authorization"}]}}`,
			field: "remoteConfig.headers",
		},
		{
			name:  "null user headers",
			data:  "multiUserConfig:\n  userDefinedHeaders: null",
			field: "multiUserConfig.userDefinedHeaders",
		},
		{
			name:  "new and old fields together",
			data:  "config: []\nenv: []",
			field: "env",
		},
		{
			name: "unknown fields and new configuration",
			data: "config:\n- key: TOKEN\n  usage: env\nunknown: true\nnpxConfig:\n  package: test\n  unknown: true",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfigurationFields([]byte(tc.data))
			if tc.field == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorContains(t, err, tc.field)
			require.ErrorContains(t, err, "top-level config")
		})
	}
}

func TestDecodeVMCPManifestRetainsConfigurationSecretBinding(t *testing.T) {
	manifest, err := DecodeVMCPManifest([]byte(`type: vmcp
displayName: RepoAccessfixedwithsecret
components:
- id: gitlab
  name: Gitlab
  mcpServerCatalogEntryKey: obot-gitlab
  configuration:
    - key: GITLAB_PERSONAL_ACCESS_TOKEN
      policy: fixed
      secretBinding:
        name: gitlab-secret
        key: gitlab_key
`))
	require.NoError(t, err)
	require.Len(t, manifest.Components, 1)
	require.Len(t, manifest.Components[0].Configuration, 1)
	require.Equal(t, &types.MCPSecretBinding{Name: "gitlab-secret", Key: "gitlab_key"}, manifest.Components[0].Configuration[0].SecretBinding)
}

func TestDecodeVMCPManifestToolSearch(t *testing.T) {
	manifest, err := DecodeVMCPManifest([]byte(`type: vmcp
displayName: Search
toolSearch: true
components:
- id: files
  name: Files
  mcpServerCatalogEntryKey: files
`))
	require.NoError(t, err)
	require.True(t, manifest.ToolSearch)
}
