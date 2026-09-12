package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMCPConvertCatalogWarnsForComposite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "composite.yaml")
	original := []byte("runtime: composite\nserverUserType: singleUser\ncompositeConfig: {}\n")
	require.NoError(t, os.WriteFile(path, original, 0o600))
	var output, warnings bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	cmd.SetErr(&warnings)
	err := (&MCPConvertCatalogYAML{}).Run(cmd, []string{path})
	require.ErrorIs(t, err, errCompositeCatalogEntry)
	require.Contains(t, warnings.String(), "Warning: "+path)
	require.Contains(t, warnings.String(), "no longer supported by catalog sync")
	require.Contains(t, output.String(), "Converted 0 catalog files.")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, data)
}

func TestMCPConvertCatalogYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entries.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`# Catalog comment
- name: Hosted
  runtime: npx
  serverUserType: multiUser
  npxConfig:
    package: example
  env:
    # Keep input metadata
    - key: TOKEN
      value: 'secret-value'
      sensitive: true
      required: true
      description: API token
    - key: FILE
      file: true
      value: |
        line one
        line two
    - key: DYNAMIC
      file: true
      dynamicFile: true
      secretBinding:
        name: credentials
        key: token
    - name: Region
      interpolated: true
      options:
        - name: Europe
          value: eu
  multiUserConfig:
    userDefinedHeaders:
      - key: X-User
        prefix: 'Bearer '
- name: Remote
  runtime: remote
  serverUserType: singleUser
  remoteConfig:
    fixedURL: https://example.com/mcp
    staticOAuthRequired: true
    headers:
      - key: Authorization
        required: true
        sensitive: true
        prefix: 'Bearer '
`), 0o640))

	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", path)
	require.NoError(t, err)
	require.Contains(t, stdout, "Converted 1 catalog files.")
	entries, array, err := mcpcatalog.DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
	require.NoError(t, err)
	require.True(t, array)
	require.Len(t, entries, 2)
	require.Len(t, entries[0].Config, 5)
	for i, usage := range []types.Usage{types.Env, types.File, types.DynamicFile, types.Interpolated, types.Header} {
		require.Equal(t, usage, entries[0].Config[i].Usage)
	}
	require.Equal(t, "secret-value", entries[0].Config[0].Value)
	require.True(t, entries[0].Config[0].Sensitive)
	require.True(t, entries[0].Config[0].Required)
	require.Equal(t, "API token", entries[0].Config[0].Description)
	require.Equal(t, "line one\nline two\n", entries[0].Config[1].Value)
	require.Equal(t, "credentials", entries[0].Config[2].SecretBinding.Name)
	require.Equal(t, "eu", entries[0].Config[3].Options[0].Value)
	require.Equal(t, "Bearer ", entries[0].Config[4].Prefix)
	require.Equal(t, types.Header, entries[1].Config[0].Usage)
	require.True(t, entries[1].RemoteConfig.StaticOAuthRequired)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(contents), "# Catalog comment")
	require.Contains(t, string(contents), "# Keep input metadata")
	require.NotContains(t, string(contents), "serverUserType")
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), info.Mode().Perm())

	stdout, err = executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", path)
	require.NoError(t, err)
	require.Contains(t, stdout, "Converted 0 catalog files.")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, contents, after)
}

func TestMCPConvertSystemCatalogYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entry.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`name: Filter
shortDescription: Filter
description: Filter
icon: icon
systemMCPServerType: filter
filterConfig:
  toolName: filter
runtime: remote
serverUserType: singleUser
env:
  - key: TOKEN
    required: true
remoteConfig:
  fixedURL: https://example.com/mcp
  headers:
    - key: Authorization
      prefix: 'Bearer '
`), 0o600))

	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", path)
	require.NoError(t, err)
	require.Contains(t, stdout, "Converted 1 catalog files.")
	entries, _, err := mcpcatalog.DecodeCatalogFile[types.SystemMCPServerCatalogEntryManifest](path, true)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, []types.MCPConfig{
		{Key: "TOKEN", Required: true, Usage: types.Env},
		{Key: "Authorization", Prefix: "Bearer ", Usage: types.Header},
	}, entries[0].Config)
	require.Equal(t, "https://example.com/mcp", entries[0].RemoteConfig.FixedURL)

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(contents), "serverUserType")
	require.NotContains(t, string(contents), "headers:")
}

func TestMCPConvertCatalogDirectory(t *testing.T) {
	dir := t.TempDir()
	legacy := []byte("name: Example\nruntime: npx\nserverUserType: singleUser\nenv:\n  - key: TOKEN\n")
	for _, name := range []string{"entry.yaml", "nested/entry.yml", "ignored/entry.yaml", "skip.yaml", ".git/entry.yaml", ".github/workflows/entry.yaml", "other.yaml", "entry.json"} {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, legacy, 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".obotcatalogs"), []byte("entry.*\nskip.yaml\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ignoreobotcatalogs"), []byte("ignored\nskip.yaml\n"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(dir, "other.yaml"), filepath.Join(dir, "nested", "entry.yaml")))
	stdout, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", dir)
	require.NoError(t, err)
	require.Contains(t, stdout, "Converted 2 catalog files.")
	for _, name := range []string{"entry.yaml", "nested/entry.yml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Contains(t, string(data), "usage: env")
	}
	for _, name := range []string{"ignored/entry.yaml", "skip.yaml", ".git/entry.yaml", ".github/workflows/entry.yaml", "other.yaml", "entry.json", "nested/entry.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, legacy, data, name)
	}
}

func TestMCPConvertCatalogYAMLRejectsWithoutWriting(t *testing.T) {
	for _, tt := range []struct {
		name string
		data string
	}{
		{
			name: "invalid YAML",
			data: "env: [",
		},
		{
			name: "multiple documents",
			data: "runtime: npx\nserverUserType: singleUser\n---\nruntime: remote\n",
		},
		{
			name: "unsupported composite",
			data: "runtime: composite\nserverUserType: singleUser\ncompositeConfig: {}\n",
		},
		{
			name: "unknown field",
			data: "runtime: npx\nserverUserType: singleUser\nunknown: value\n",
		},
		{
			name: "duplicate config keys",
			data: "env: [{key: TOKEN}]\nremoteConfig:\n  headers: [{key: TOKEN}]\n",
		},
		{
			name: "invalid second entry",
			data: "- runtime: npx\n  serverUserType: singleUser\n- env: invalid\n",
		},
		{
			name: "duplicate YAML keys",
			data: "runtime: npx\nruntime: remote\nserverUserType: singleUser\n",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "entry.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tt.data), 0o600))
			_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", path)
			require.Error(t, err)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, tt.data, string(data))
		})
	}
}

func TestMCPConvertCatalogYAMLPartialMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog")
	require.NoError(t, os.WriteFile(path, []byte(`runtime: npx
config:
  - key: EXISTING
    usage: interpolated
env:
  - key: NEW
    file: false
remoteConfig:
  headers: null
serverUserType: singleUser
`), 0o600))
	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", path)
	require.NoError(t, err)
	entries, _, err := mcpcatalog.DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
	require.NoError(t, err)
	require.Len(t, entries[0].Config, 2)
	require.Equal(t, types.Interpolated, entries[0].Config[0].Usage)
	require.Equal(t, types.Env, entries[0].Config[1].Usage)
}

func TestMCPConvertCatalogYAMLRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entry.yaml")
	data := []byte("runtime: npx\nserverUserType: singleUser\n")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	link := filepath.Join(dir, "link.yaml")
	require.NoError(t, os.Symlink(path, link))
	_, err := executeMCPTestCommand(t, mcpTestRoot("http://unused.example"), "convert-catalog-yaml", link)
	require.ErrorContains(t, err, "symlinks are not converted")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, data, after)
}
