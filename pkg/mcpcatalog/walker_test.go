package mcpcatalog

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestWalkCatalogFilesSkipsSymlinksAndIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte("name: Valid\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ignored.yaml"), []byte("name: Ignored\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".ignoreobotcatalogs"), []byte("ignored.yaml\n"), 0o600))
	require.NoError(t, os.Symlink(validPath, filepath.Join(dir, "linked.yaml")))

	files, _, err := WalkCatalogFiles(dir)
	require.NoError(t, err)
	var paths []string
	for path, err := range files {
		require.NoError(t, err)
		paths = append(paths, path)
	}
	require.Equal(t, []string{validPath}, paths)
}

func TestWalkCatalogFilesFallsBackWhenPatternFilesCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte("name: Valid\n"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".obotcatalogs"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".ignoreobotcatalogs"), 0o700))

	files, usingObotCatalogsFile, err := WalkCatalogFiles(dir)
	require.NoError(t, err)
	require.False(t, usingObotCatalogsFile)

	var paths []string
	for path, err := range files {
		require.NoError(t, err)
		paths = append(paths, path)
	}
	require.Equal(t, []string{validPath}, paths)
}

func TestWalkCatalogFilesFallsBackWhenPatternLineIsTooLong(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "valid.yaml")
	require.NoError(t, os.WriteFile(validPath, []byte("name: Valid\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".obotcatalogs"), []byte(strings.Repeat("x", bufio.MaxScanTokenSize+1)), 0o600))

	files, usingObotCatalogsFile, err := WalkCatalogFiles(dir)
	require.NoError(t, err)
	require.True(t, usingObotCatalogsFile)

	var paths []string
	for path, err := range files {
		require.NoError(t, err)
		paths = append(paths, path)
	}
	require.Equal(t, []string{validPath}, paths)
}

func TestDecodeCatalogFileStrictness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "entry.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`name: First
name: Second
runtime: npx
npxConfig:
  package: test
`), 0o600))

	entries, isArray, err := DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, false)
	require.NoError(t, err)
	require.False(t, isArray)
	require.Len(t, entries, 1)
	require.Equal(t, "Second", entries[0].Name)

	_, _, err = DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
	require.ErrorContains(t, err, `key "name" already set`)
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

func TestNormalizeSystemManifest(t *testing.T) {
	entry := types.SystemMCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		Env:          []types.MCPEnv{{MCPHeader: types.MCPHeader{Name: "config-file.json"}}},
		RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Name: "api_key"}}},
	}

	NormalizeSystemManifest(&entry)

	require.Equal(t, types.ServerUserTypeSingleUser, entry.ServerUserType)
	require.Equal(t, "CONFIG_FILE_JSON", entry.Env[0].Key)
	require.True(t, entry.Env[0].File)
	require.Equal(t, "API-KEY", entry.RemoteConfig.Headers[0].Key)
}
