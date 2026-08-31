package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListEntriesFromAllSourcesMinimal(t *testing.T) {
	entry := minimalResponseTestEntry()
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&entry).
		Build())
	request := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries?all=true&minimal=true", nil)
	recorder := httptest.NewRecorder()

	err := (&MCPHandler{serverURL: "https://example.com"}).ListEntriesFromAllSources(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        storageClient,
		User: &kuser.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response types.MCPServerCatalogEntryList
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	manifest := response.Items[0].Manifest
	require.Equal(t, map[string]string{"category": "developer-tools"}, manifest.Metadata)
	require.Equal(t, "test-entry-key", manifest.EntryKey)
	require.Empty(t, manifest.Description)
	require.Empty(t, manifest.ToolPreview)
	require.Equal(t, "Upgrade note", manifest.UpgradeNote)
	require.Empty(t, manifest.RepoURL)
	require.Equal(t, "https://example.com/mcp", manifest.RemoteConfig.FixedURL)
	require.Equal(t, "Test Entry", manifest.Name)
	require.Equal(t, "Short description", manifest.ShortDescription)
	require.Equal(t, "https://example.com/icon.svg", manifest.Icon)
	require.Equal(t, types.RuntimeRemote, manifest.Runtime)
	require.Equal(t, types.ServerUserTypeSingleUser, manifest.ServerUserType)

	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"metadata": {"category": "developer-tools"},
		"entryKey": "test-entry-key",
		"name": "Test Entry",
		"shortDescription": "Short description",
		"description": "",
		"icon": "https://example.com/icon.svg",
		"runtime": "remote",
		"remoteConfig": {"fixedURL": "https://example.com/mcp"},
		"serverUserType": "singleUser",
		"upgradeNote": "Upgrade note"
	}`, string(manifestJSON))
}

func TestMinimalCatalogEntryPreservesLaunchConfiguration(t *testing.T) {
	entry := minimalResponseTestEntry()
	entry.Spec.Manifest.UVXConfig = &types.UVXRuntimeConfig{Package: "python-package"}
	entry.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{Package: "node-package"}
	entry.Spec.Manifest.ContainerizedConfig = &types.ContainerizedRuntimeConfig{Image: "example/image", Port: 8080, Path: "/mcp"}
	entry.Spec.Manifest.CompositeConfig = &types.CompositeCatalogConfig{}
	entry.Spec.Manifest.MultiUserConfig = &types.MultiUserConfig{}
	entry.Spec.Manifest.Env = []types.MCPEnv{{Name: "Token", Key: "TOKEN"}}
	entry.Spec.Manifest.Resources = &types.MCPResourceRequirements{}

	expected := entry.Spec.Manifest
	expected.Description = ""
	expected.ToolPreview = nil
	expected.RepoURL = ""

	actual := convertMCPServerCatalogEntryForList(entry, "", "", "", true)
	require.Equal(t, expected, actual.Manifest)
}

func TestMinimalCatalogEntryMinimizesCompositeComponents(t *testing.T) {
	entry := minimalResponseTestEntry()
	component := minimalResponseTestEntry().Spec.Manifest
	nestedComponent := minimalResponseTestEntry().Spec.Manifest
	component.Runtime = types.RuntimeComposite
	component.CompositeConfig = &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{{
		Manifest: nestedComponent,
	}}}
	entry.Spec.Manifest.Runtime = types.RuntimeComposite
	entry.Spec.Manifest.CompositeConfig = &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{{
		Manifest: component,
	}}}

	actual := convertMCPServerCatalogEntryForList(entry, "", "", "", true)
	component = actual.Manifest.CompositeConfig.ComponentServers[0].Manifest
	nestedComponent = component.CompositeConfig.ComponentServers[0].Manifest

	for _, manifest := range []types.MCPServerCatalogEntryManifest{actual.Manifest, component, nestedComponent} {
		require.Empty(t, manifest.Description)
		require.Empty(t, manifest.ToolPreview)
		require.Empty(t, manifest.RepoURL)
		require.Equal(t, "Upgrade note", manifest.UpgradeNote)
	}
}

func TestListEntriesFromAllSourcesFullByDefault(t *testing.T) {
	entry := minimalResponseTestEntry()
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&entry).
		Build())
	request := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries?all=true", nil)
	recorder := httptest.NewRecorder()

	err := (&MCPHandler{serverURL: "https://example.com"}).ListEntriesFromAllSources(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        storageClient,
		User: &kuser.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
		},
	})
	require.NoError(t, err)

	var response types.MCPServerCatalogEntryList
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	require.Equal(t, "Full description", response.Items[0].Manifest.Description)
	require.Len(t, response.Items[0].Manifest.ToolPreview, 1)
	require.Equal(t, "Upgrade note", response.Items[0].Manifest.UpgradeNote)
	require.Equal(t, "https://example.com/repo", response.Items[0].Manifest.RepoURL)
	require.Equal(t, "https://example.com/mcp", response.Items[0].Manifest.RemoteConfig.FixedURL)
}

func TestListCatalogEntriesMinimal(t *testing.T) {
	entry := minimalResponseTestEntry()
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.MCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace}, &entry).
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerCatalogEntry).Spec.MCPCatalogName}
		}).
		Build())
	request := httptest.NewRequest(http.MethodGet, "/api/mcp-catalogs/default/entries?all=true&minimal=TRUE", nil)
	request.SetPathValue("catalog_id", system.DefaultCatalog)
	recorder := httptest.NewRecorder()

	err := (&MCPCatalogHandler{serverURL: "https://example.com"}).ListEntries(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		Storage:        storageClient,
		User: &kuser.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
		},
	})
	require.NoError(t, err)

	var response types.MCPServerCatalogEntryList
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	require.Empty(t, response.Items[0].Manifest.Description)
}

func minimalResponseTestEntry() v1.MCPServerCatalogEntry {
	return v1.MCPServerCatalogEntry{
		Name:      "test-entry",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
			Manifest: types.MCPServerCatalogEntryManifest{
				Metadata:         map[string]string{"category": "developer-tools"},
				EntryKey:         "test-entry-key",
				Name:             "Test Entry",
				ShortDescription: "Short description",
				Description:      "Full description",
				Icon:             "https://example.com/icon.svg",
				RepoURL:          "https://example.com/repo",
				UpgradeNote:      "Upgrade note",
				ToolPreview: []types.MCPServerTool{
					{Name: "tool", Description: "Tool description"},
				},
				Runtime:        types.RuntimeRemote,
				ServerUserType: types.ServerUserTypeSingleUser,
				RemoteConfig:   &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
			},
		},
	}
}
