package mcpcatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestValidatePathsDirectory(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte(`name: Remote
entryKey: remote
shortDescription: Remote server
description: Remote server
icon: icon
runtime: remote
remoteConfig:
  fixedURL: https://does-not-resolve.invalid/mcp
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.yaml"), []byte("not: a catalog entry\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ignoreobotcatalogs"), []byte("ignored.yaml\n"), 0o600))

	summary, err := ValidatePaths(t.Context(), []string{dir, validPath}, LocalValidationOptions{})
	require.NoError(t, err)
	require.Equal(t, ValidationSummary{Files: 1, Entries: 1}, summary)
}

func TestValidatePathsSupportsEntryArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entries.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
- name: First
  entryKey: first
  shortDescription: First
  description: First
  icon: icon
  runtime: npx
  npxConfig:
    package: first
- name: Second
  entryKey: second
  shortDescription: Second
  description: Second
  icon: icon
  runtime: uvx
  uvxConfig:
    package: second
`), 0o600))

	summary, err := ValidatePaths(t.Context(), []string{path}, LocalValidationOptions{})
	require.NoError(t, err)
	require.Equal(t, ValidationSummary{Files: 1, Entries: 2}, summary)
}

func TestValidatePathsAggregatesErrors(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "duplicate-key.yaml", `name: First
name: Second
runtime: npx
`)
	writeTestFile(t, dir, "unknown-field.yaml", `name: Unknown
entryKey: shared
shortDescription: Unknown
description: Unknown
icon: icon
runtime: npx
npxConfig:
  package: test
unknownField: true
`)
	writeTestFile(t, dir, "invalid-runtime.yaml", `name: Invalid
entryKey: shared
shortDescription: Invalid
description: Invalid
icon: icon
runtime: invalid
`)
	writeTestFile(t, dir, "duplicate-entry-key.yaml", `name: Duplicate
entryKey: shared
shortDescription: Duplicate
description: Duplicate
icon: icon
runtime: npx
npxConfig:
  package: test
`)

	summary, err := ValidatePaths(t.Context(), []string{dir}, LocalValidationOptions{})
	require.Error(t, err)
	require.Equal(t, 4, summary.Files)
	for _, expected := range []string{
		"duplicate-key.yaml",
		"key \"name\" already set",
		"unknown-field.yaml",
		"unknown field \"unknownField\"",
		"unsupported runtime",
		"duplicate source entry key \"shared\"",
	} {
		require.Contains(t, err.Error(), expected)
	}
}

func TestValidatePathsRequiresEntryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entries.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
- name: Missing
  shortDescription: Missing
  description: Missing
  icon: icon
  runtime: npx
  npxConfig:
    package: missing
- name: Whitespace
  entryKey: "  "
  shortDescription: Whitespace
  description: Whitespace
  icon: icon
  runtime: npx
  npxConfig:
    package: whitespace
`), 0o600))

	_, err := ValidatePaths(t.Context(), []string{path}, LocalValidationOptions{RequireEntryKey: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "entries.yaml[0]: entryKey is required")
	require.Contains(t, err.Error(), "entries.yaml[1]: entryKey is required")
}

func TestNormalizeManifest(t *testing.T) {
	entry := types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		Env:          []types.MCPEnv{{MCPHeader: types.MCPHeader{Name: "config-file.json"}}},
		RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Name: "api_key"}}},
	}

	NormalizeManifest(&entry)

	require.Equal(t, types.ServerUserTypeSingleUser, entry.ServerUserType)
	require.Equal(t, "CONFIG_FILE_JSON", entry.Env[0].Key)
	require.True(t, entry.Env[0].File)
	require.Equal(t, "API-KEY", entry.RemoteConfig.Headers[0].Key)
}

func writeTestFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(strings.TrimSpace(contents)+"\n"), 0o600))
}
