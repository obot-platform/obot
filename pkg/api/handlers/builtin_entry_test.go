package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Only admins and owners can use built-in servers, so only they see their entries.
func TestBuiltinEntriesAreHiddenFromNonAdmins(t *testing.T) {
	builtin := v1.MCPServerCatalogEntry{
		Name: "default-obot-admin", Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:          "Obot Admin",
				Runtime:       types.RuntimeBuiltin,
				BuiltinConfig: &types.BuiltinRuntimeConfig{Name: "obot-admin"},
			},
		},
	}
	remote := minimalResponseTestEntry()
	storageClient := storage.Client(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&builtin, &remote).Build())

	tests := []struct {
		name        string
		groups      []string
		entry       v1.MCPServerCatalogEntry
		wantVisible bool
	}{
		{
			name:        "admin sees the built-in entry",
			groups:      types.RoleAdmin.Groups(),
			entry:       builtin,
			wantVisible: true,
		},
		{
			name:        "owner sees the built-in entry",
			groups:      types.RoleOwner.Groups(),
			entry:       builtin,
			wantVisible: true,
		},
		{
			name:   "basic user doesn't",
			groups: types.RoleBasic.Groups(),
			entry:  builtin,
		},
		{
			name:   "power user doesn't",
			groups: types.RolePowerUserPlus.Groups(),
			entry:  builtin,
		},
		{
			name:        "basic user still sees other entries",
			groups:      types.RoleBasic.Groups(),
			entry:       remote,
			wantVisible: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries/"+tt.entry.Name, nil)
			request.SetPathValue("entry_id", tt.entry.Name)
			req := api.Context{
				Request:        request,
				ResponseWriter: httptest.NewRecorder(),
				Storage:        storageClient,
				User:           &kuser.DefaultInfo{Name: "user", UID: "1", Groups: tt.groups},
			}
			assert.Equal(t, !tt.wantVisible, hiddenBuiltinEntry(req, tt.entry))

			err := (&MCPHandler{serverURL: "https://example.com"}).GetEntryFromAllSources(req)
			if tt.wantVisible {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, "not found")
			}
		})
	}
}
