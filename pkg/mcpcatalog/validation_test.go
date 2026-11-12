package mcpcatalog

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

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

func TestDecodeVMCPManifestAgentSubjects(t *testing.T) {
	for _, tt := range []struct {
		name      string
		subject   string
		wantError bool
	}{
		{
			name:    "every agent",
			subject: `{type: agent, id: "*"}`,
		},
		{
			name:    "everyone",
			subject: `{type: selector, id: "*"}`,
		},
		{
			name:    "every user",
			subject: `{type: user, id: "*"}`,
		},
		{
			name:      "one agent",
			subject:   `{type: agent, id: "7"}`,
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeVMCPManifest([]byte(`type: vmcp
displayName: Agents
components:
- id: files
  name: Files
  mcpServerCatalogEntryKey: files
profiles:
- name: agents
  subjects:
  - ` + tt.subject + `
`))
			if tt.wantError {
				require.ErrorContains(t, err, "cannot name an agent")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDecodeVMCPManifestProfileIDs(t *testing.T) {
	for _, tt := range []struct {
		name      string
		id        string
		wantError bool
	}{
		{
			name: "no ID",
		},
		{
			name: "explicit ID",
			id:   "engineering-agents",
		},
		{
			name:      "uppercase ID",
			id:        "Engineering",
			wantError: true,
		},
		{
			name:      "ID with a slash",
			id:        "a/b",
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			id := ""
			if tt.id != "" {
				id = "\n  id: " + tt.id
			}
			manifest, err := DecodeVMCPManifest([]byte(`type: vmcp
displayName: Profiles
components:
- id: files
  name: Files
  mcpServerCatalogEntryKey: files
profiles:
- name: engineering` + id + `
  subjects:
  - {type: group, id: engineering}
`))
			// Decoding keeps the ID, and manifest validation, which every caller runs, checks it.
			require.NoError(t, err)
			require.Equal(t, tt.id, manifest.Profiles[0].ID)
			if tt.wantError {
				require.ErrorContains(t, manifest.Validate(), "invalid profile ID")
			}
		})
	}
}
