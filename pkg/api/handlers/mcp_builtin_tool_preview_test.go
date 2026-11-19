package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuiltinEntryToolPreviewUsesStoredTools(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "default-obot-admin",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:          "Obot Admin",
				Runtime:       types.RuntimeBuiltin,
				BuiltinConfig: &types.BuiltinRuntimeConfig{Name: "obot-admin"},
				ToolPreview:   []types.MCPServerTool{{ID: "list_catalogs", Name: "list_catalogs"}},
			},
		},
	}
	store := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry,
		&v1.MCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace}).Build()
	tests := []struct {
		name      string
		query     string
		groups    []string
		wantError string
	}{
		{
			name:   "CLI confirmation reads authoritative tools",
			query:  "?dryRun=true",
			groups: []string{types.GroupAdmin},
		},
		{
			name:      "builtin remains immutable",
			groups:    []string{types.GroupAdmin},
			wantError: "entry is not editable",
		},
		{
			name:      "non-admin cannot read builtin entry tools",
			query:     "?dryRun=true",
			groups:    []string{types.GroupBasic},
			wantError: "not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/mcp-catalogs/default/entries/default-obot-admin/generate-tool-previews"+tt.query, nil)
			request.SetPathValue("catalog_id", system.DefaultCatalog)
			request.SetPathValue("entry_id", entry.Name)
			recorder := httptest.NewRecorder()
			// No gateway, OAuth checker or session manager: this must only read storage.
			err := (&MCPCatalogHandler{}).GenerateToolPreviews(api.Context{
				Request:        request,
				ResponseWriter: recorder,
				Storage:        store,
				User:           &user.DefaultInfo{UID: "caller", Groups: tt.groups},
			})
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			var result types.MCPServerCatalogEntry
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &result))
			require.Equal(t, entry.Spec.Manifest.ToolPreview, result.Manifest.ToolPreview)
			var stored v1.MCPServerCatalogEntry
			require.NoError(t, store.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &stored))
			require.Equal(t, entry.Spec, stored.Spec)
			require.Equal(t, entry.Status, stored.Status)
		})
	}
}
