package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func catalogEntryRequest(t *testing.T, storage kclient.WithWatch, gatewayClient *gatewayclient.Client, method, path string, body any, pathValues map[string]string) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	for key, value := range pathValues {
		req.SetPathValue(key, value)
	}
	recorder := httptest.NewRecorder()
	return api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           testUserWithRole("admin", types.GroupAdmin),
	}, recorder
}

func TestCatalogEntryStaticConfigurationIsStoredInCredential(t *testing.T) {
	storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace})
	gatewayClient := newEnforcementTestGatewayClient(t)
	handler := &MCPCatalogHandler{mcpBackend: "docker", sessionManager: &mcp.SessionManager{}}

	manifest := types.MCPServerCatalogEntryManifest{
		Name:      "static-entry",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config: []types.MCPConfig{
			{Key: "api-token", Name: "API token", Value: "secret", Sensitive: true, Required: true, Usage: types.Env},
			{Key: "USER", Name: "User", Usage: types.Env},
		},
		// Clients cannot choose the revision.
		StaticConfigurationRevision: "client-chosen",
	}
	req, recorder := catalogEntryRequest(t, storage, gatewayClient, http.MethodPost, "/api/mcp-catalogs/catalog-1/entries", manifest, map[string]string{"catalog_id": "catalog-1"})
	require.NoError(t, handler.CreateEntry(req))
	assert.NotContains(t, recorder.Body.String(), "secret")

	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, storage.List(t.Context(), &entries))
	require.Len(t, entries.Items, 1)
	entry := entries.Items[0]
	// The API normalizes keys the same way catalog sync does.
	require.Equal(t, "API_TOKEN", entry.Spec.Manifest.Config[0].Key)
	assert.True(t, entry.Spec.Manifest.Config[0].Static)
	assert.Empty(t, entry.Spec.Manifest.Config[0].Value)
	assert.False(t, entry.Spec.Manifest.Config[1].Static)
	revision := entry.Spec.Manifest.StaticConfigurationRevision
	require.NotEmpty(t, revision)
	require.NotEqual(t, "client-chosen", revision)

	reveal := func() map[string]string {
		t.Helper()
		req, recorder := catalogEntryRequest(t, storage, gatewayClient, http.MethodPost, "/api/mcp-catalogs/catalog-1/entries/"+entry.Name+"/reveal", nil, map[string]string{"catalog_id": "catalog-1", "entry_id": entry.Name})
		require.NoError(t, handler.RevealEntry(req))
		var values map[string]string
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &values))
		return values
	}
	assert.Equal(t, map[string]string{"API_TOKEN": "secret"}, reveal())

	update := func(manifest types.MCPServerCatalogEntryManifest) v1.MCPServerCatalogEntry {
		t.Helper()
		req, recorder := catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-catalogs/catalog-1/entries/"+entry.Name, manifest, map[string]string{"catalog_id": "catalog-1", "entry_id": entry.Name})
		require.NoError(t, handler.UpdateEntry(req))
		assert.NotContains(t, recorder.Body.String(), "secret")
		var updated v1.MCPServerCatalogEntry
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(&entry), &updated))
		return updated
	}

	// A static field submitted without a value keeps its stored value.
	updated := update(entry.Spec.Manifest)
	assert.Equal(t, revision, updated.Spec.Manifest.StaticConfigurationRevision)
	assert.Equal(t, map[string]string{"API_TOKEN": "secret"}, reveal())

	rotate := *updated.Spec.Manifest.DeepCopy()
	rotate.Config[0].Value = "rotated"
	updated = update(rotate)
	assert.NotEqual(t, revision, updated.Spec.Manifest.StaticConfigurationRevision)
	assert.True(t, updated.Spec.Manifest.Config[0].Static)
	assert.Empty(t, updated.Spec.Manifest.Config[0].Value)
	assert.Equal(t, map[string]string{"API_TOKEN": "rotated"}, reveal())

	userSupplied := *updated.Spec.Manifest.DeepCopy()
	userSupplied.Config[0].Static = false
	updated = update(userSupplied)
	assert.Empty(t, updated.Spec.Manifest.StaticConfigurationRevision)
	assert.Empty(t, reveal())
}

func TestCreateCatalogEntryRejectsStaticFieldWithoutValue(t *testing.T) {
	storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace})
	manifest := types.MCPServerCatalogEntryManifest{
		Name:      "static-entry",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config:    []types.MCPConfig{{Key: "API_TOKEN", Static: true, Usage: types.Env}},
	}
	req, _ := catalogEntryRequest(t, storage, newEnforcementTestGatewayClient(t), http.MethodPost, "/api/mcp-catalogs/catalog-1/entries", manifest, map[string]string{"catalog_id": "catalog-1"})

	err := (&MCPCatalogHandler{mcpBackend: "docker", sessionManager: &mcp.SessionManager{}}).CreateEntry(req)
	require.ErrorContains(t, err, `static configuration "API_TOKEN" requires a value`)

	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, storage.List(t.Context(), &entries))
	assert.Empty(t, entries.Items)
}

func TestUpdateCatalogEntryKeepsStaticValuesStoredBeforeNormalization(t *testing.T) {
	gatewayClient := newEnforcementTestGatewayClient(t)
	// Entries created through the API before it normalized manifests keep their original keys.
	stored := types.MCPServerCatalogEntryManifest{
		Name:      "static-entry",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
		Config:    []types.MCPConfig{{Key: "api-token", Value: "secret", Required: true, Usage: types.Env}},
	}
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry1static",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{MCPCatalogName: "catalog-1", Editable: true},
	}
	require.NoError(t, mcp.StoreStaticConfiguration(t.Context(), gatewayClient, entry.Name, &stored, ""))
	entry.Spec.Manifest = stored
	storage := newFakeStorage(t, &v1.MCPCatalog{Name: "catalog-1", Namespace: system.DefaultNamespace}, entry)
	handler := &MCPCatalogHandler{mcpBackend: "docker", sessionManager: &mcp.SessionManager{}}

	req, _ := catalogEntryRequest(t, storage, gatewayClient, http.MethodPut, "/api/mcp-catalogs/catalog-1/entries/"+entry.Name, stored, map[string]string{"catalog_id": "catalog-1", "entry_id": entry.Name})
	require.NoError(t, handler.UpdateEntry(req))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &updated))
	require.Equal(t, "API_TOKEN", updated.Spec.Manifest.Config[0].Key)
	assert.True(t, updated.Spec.Manifest.Config[0].Static)
	values, err := mcp.RevealStaticConfiguration(t.Context(), gatewayClient, entry.Name, updated.Spec.Manifest.StaticConfigurationRevision)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"API_TOKEN": "secret"}, values)
}
