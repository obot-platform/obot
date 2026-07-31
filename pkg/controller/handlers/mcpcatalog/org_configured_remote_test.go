package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestReadMCPCatalogAcceptsOrgConfiguredRemoteShapes(t *testing.T) {
	dir := t.TempDir()
	manifests := map[string]string{
		"github.yaml": `name: GitHub
entryKey: obot-github
serverUserType: multiUser
shortDescription: GitHub
description: GitHub
runtime: remote
remoteConfig:
  hostname: api.githubcopilot.com
  headers:
    - name: Personal Access Token
      key: Authorization
      required: true
      sensitive: true
`,
		"databricks.yaml": `name: Databricks Genie Spaces
entryKey: obot-databricks-genie-spaces
serverUserType: multiUser
shortDescription: Databricks
description: Databricks
metadata:
  allow-multiple: "true"
runtime: remote
remoteConfig:
  URLTemplate: ${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}
  headers:
    - name: Personal Access Token
      key: Authorization
      required: true
      sensitive: true
      prefix: "Bearer "
env:
  - name: Databricks workspace hostname
    key: DATABRICKS_WORKSPACE_URL
    required: true
  - name: Genie space ID
    key: DATABRICKS_GENIE_SPACE_ID
    required: true
`,
		"google-maps.yaml": `name: Google Maps Grounding Lite
entryKey: obot-google-maps-grounding-lite
serverUserType: multiUser
shortDescription: Google Maps
description: Google Maps
metadata:
  allow-multiple: "true"
runtime: remote
remoteConfig:
  fixedURL: https://mapstools.googleapis.com/mcp
  headers:
    - name: API Key
      key: X-Goog-Api-Key
      required: true
      sensitive: true
`,
	}

	for name, manifest := range manifests {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(manifest), 0o600))
	}

	objects, err := (&Handler{}).readMCPCatalog(t.Context(), "default", dir, "")
	require.NoError(t, err)
	require.Len(t, objects, len(manifests))

	entries := make(map[string]types.MCPServerCatalogEntryManifest, len(objects))
	for _, object := range objects {
		entry, ok := object.(*v1.MCPServerCatalogEntry)
		require.True(t, ok, "unexpected catalog object %T", object)
		require.Equal(t, types.ServerUserTypeMultiUser, entry.Spec.Manifest.ServerUserType)
		entries[entry.Spec.Manifest.EntryKey] = entry.Spec.Manifest
	}

	github := entries["obot-github"]
	require.Equal(t, "api.githubcopilot.com", github.RemoteConfig.Hostname)
	require.Len(t, github.RemoteConfig.Headers, 1)
	require.Equal(t, "AUTHORIZATION", github.RemoteConfig.Headers[0].Key)
	require.True(t, github.RemoteConfig.Headers[0].Required)
	require.True(t, github.RemoteConfig.Headers[0].Sensitive)

	databricks := entries["obot-databricks-genie-spaces"]
	require.Equal(t, "true", databricks.Metadata["allow-multiple"])
	require.Equal(
		t,
		"${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}",
		databricks.RemoteConfig.URLTemplate,
	)
	require.Len(t, databricks.RemoteConfig.Headers, 1)
	require.Equal(t, "AUTHORIZATION", databricks.RemoteConfig.Headers[0].Key)
	require.True(t, databricks.RemoteConfig.Headers[0].Required)
	require.True(t, databricks.RemoteConfig.Headers[0].Sensitive)
	require.Equal(t, "Bearer ", databricks.RemoteConfig.Headers[0].Prefix)
	require.Equal(t, "DATABRICKS_WORKSPACE_URL", databricks.Env[0].Key)
	require.Equal(t, "DATABRICKS_GENIE_SPACE_ID", databricks.Env[1].Key)

	googleMaps := entries["obot-google-maps-grounding-lite"]
	require.Equal(t, "true", googleMaps.Metadata["allow-multiple"])
	require.Equal(t, "https://mapstools.googleapis.com/mcp", googleMaps.RemoteConfig.FixedURL)
	require.Len(t, googleMaps.RemoteConfig.Headers, 1)
	require.Equal(t, "X-GOOG-API-KEY", googleMaps.RemoteConfig.Headers[0].Key)
	require.True(t, googleMaps.RemoteConfig.Headers[0].Required)
	require.True(t, googleMaps.RemoteConfig.Headers[0].Sensitive)
}
