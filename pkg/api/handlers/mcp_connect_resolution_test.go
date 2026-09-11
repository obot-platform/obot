package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCatalogConnectIDExcludesVMCPComponents(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec v1.MCPServerSpec
	}{
		{
			name: "vMCP",
			spec: v1.MCPServerSpec{VMCPID: "vmcp1parent"},
		},
		{
			name: "vMCP instance",
			spec: v1.MCPServerSpec{VMCPInstanceID: "vmcpi1parent"},
		},
	} {
		for _, existing := range []bool{false, true} {
			name := "creates standalone"
			if existing {
				name = "selects standalone"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				entry := &v1.MCPServerCatalogEntry{
					Name:      "entry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{Manifest: types.MCPServerCatalogEntryManifest{
						Runtime:   types.RuntimeNPX,
						NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
					}},
				}
				component := &v1.MCPServer{
					Name:              "ms1component",
					Namespace:         system.DefaultNamespace,
					CreationTimestamp: metav1.NewTime(time.Unix(1, 0)),
					Spec:              tc.spec,
				}
				component.Spec.UserID = "user"
				component.Spec.MCPServerCatalogEntryName = entry.Name
				standalone := &v1.MCPServer{
					Name:              "ms1standalone",
					Namespace:         system.DefaultNamespace,
					CreationTimestamp: metav1.NewTime(time.Unix(2, 0)),
					Spec: v1.MCPServerSpec{
						UserID:                    "user",
						MCPServerCatalogEntryName: entry.Name,
					},
				}
				objects := []kclient.Object{entry, component}
				if existing {
					objects = append(objects, standalone)
				}
				storage := newFakeStorage(t, objects...)
				ctx := api.Context{
					Request: httptest.NewRequest(http.MethodGet, "/mcp-connect/entry", nil),
					Storage: storage,
					User:    testUser("user"),
				}
				server, instance, err := mcpServerOrInstanceFromConnectURL(ctx, entry.Name, "", mcp.ValidationOptions{})
				require.NoError(t, err)
				require.Empty(t, instance.Name)
				require.NotEmpty(t, server.Name)
				require.NotEqual(t, component.Name, server.Name)
				require.Empty(t, server.Spec.VMCPID)
				require.Empty(t, server.Spec.VMCPInstanceID)
				if existing {
					require.Equal(t, standalone.Name, server.Name)
				}
				slug, err := SlugForMCPServer(t.Context(), storage, server, "user", "", "")
				require.NoError(t, err)
				require.Equal(t, entry.Name, slug)
				slug, err = SlugForMCPServer(t.Context(), storage, *component, "user", "", "")
				require.NoError(t, err)
				require.Equal(t, component.Name, slug)
				require.Equal(t, []string{system.MCPConnectURL("https://obot.example.com", component.Name)}, component.ValidConnectURLs("https://obot.example.com"))
			})
		}
	}
}
