package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestCatalogEntriesRejectCompositeRuntime(t *testing.T) {
	for _, scope := range []string{"catalog_id", "workspace_id"} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			t.Run(scope+"/"+method, func(t *testing.T) {
				entry := &v1.MCPServerCatalogEntry{
					Name:      "entry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						Editable: true,
						Manifest: types.MCPServerCatalogEntryManifest{
							Name:      "original",
							Runtime:   types.RuntimeNPX,
							NPXConfig: &types.NPXRuntimeConfig{Package: "test-server"},
						},
					},
				}
				if scope == "catalog_id" {
					entry.Spec.MCPCatalogName = "scope"
				} else {
					entry.Spec.PowerUserWorkspaceID = "scope"
				}
				storage := newFakeStorage(t, entry,
					&v1.MCPCatalog{Name: "scope", Namespace: system.DefaultNamespace},
					&v1.PowerUserWorkspace{Name: "scope", Namespace: system.DefaultNamespace},
				)
				req := httptest.NewRequest(method, "/", strings.NewReader(`{"name":"composite","runtime":"composite","compositeConfig":{"componentServers":[]}}`))
				req.SetPathValue(scope, "scope")
				req.SetPathValue("entry_id", entry.Name)
				ctx := api.Context{
					ResponseWriter: httptest.NewRecorder(),
					Request:        req,
					Storage:        storage,
					User:           testUserWithRole("admin", types.GroupAdmin),
				}
				handler := &MCPCatalogHandler{sessionManager: &mcp.SessionManager{}}
				action := handler.CreateEntry
				if method == http.MethodPut {
					action = handler.UpdateEntry
				}
				require.ErrorContains(t, action(ctx), "unsupported runtime")
				var entries v1.MCPServerCatalogEntryList
				require.NoError(t, storage.List(t.Context(), &entries))
				require.Len(t, entries.Items, 1)
				require.Equal(t, entry.Spec.Manifest, entries.Items[0].Spec.Manifest)
			})
		}
	}
}
