package mcpcatalog

import (
	"testing"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestDeleteUnauthorizedSkipsAgentOwners checks that the access control cleanup controllers never look up an agent
// owner as a user. Agents own only vMCP component servers and connections, which these controllers skip already; an
// agent-owned server or connection outside a vMCP breaks that invariant, and is skipped rather than failing every
// retry. The handler has no gateway client or rule helper, so any lookup for the agent would fail the test.
func TestDeleteUnauthorizedSkipsAgentOwners(t *testing.T) {
	index := func(builder *fake.ClientBuilder, object kclient.Object, field string) *fake.ClientBuilder {
		return builder.WithIndex(object, field, func(o kclient.Object) []string {
			return []string{o.(fields.Fields).Get(field)}
		})
	}
	newClient := func(objects ...kclient.Object) kclient.WithWatch {
		builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...)
		builder = index(builder, &v1.AccessControlRule{}, "spec.mcpCatalogID")
		builder = index(builder, &v1.AccessControlRule{}, "spec.powerUserWorkspaceID")
		builder = index(builder, &v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName")
		builder = index(builder, &v1.MCPServerCatalogEntry{}, "spec.powerUserWorkspaceID")
		builder = index(builder, &v1.MCPServer{}, "spec.mcpServerCatalogEntryName")
		builder = index(builder, &v1.MCPServer{}, "spec.mcpCatalogID")
		builder = index(builder, &v1.MCPServer{}, "spec.powerUserWorkspaceID")
		builder = index(builder, &v1.MCPServerInstance{}, "spec.mcpServerName")
		return builder.Build()
	}

	catalog := &v1.MCPCatalog{Name: "default", Namespace: "default"}
	workspace := &v1.PowerUserWorkspace{Name: "puw1owner", Namespace: "default"}
	catalogEntry := &v1.MCPServerCatalogEntry{Name: "entry", Namespace: "default", Spec: v1.MCPServerCatalogEntrySpec{MCPCatalogName: catalog.Name}}
	workspaceEntry := &v1.MCPServerCatalogEntry{Name: "workspace-entry", Namespace: "default", Spec: v1.MCPServerCatalogEntrySpec{PowerUserWorkspaceID: workspace.Name}}

	h := &Handler{}
	for _, tc := range []struct {
		name    string
		handler router.HandlerFunc
		parent  kclient.Object
		owned   kclient.Object
		objects []kclient.Object
	}{
		{
			name:    "catalog servers",
			handler: h.DeleteUnauthorizedMCPServersForCatalog,
			parent:  catalog,
			owned: &v1.MCPServer{Name: "ms1agent", Namespace: "default", Spec: v1.MCPServerSpec{
				UserID:                    "agent:7",
				VMCPInstanceID:            "vmcpi1agent",
				MCPServerCatalogEntryName: catalogEntry.Name,
			}},
			objects: []kclient.Object{catalogEntry},
		},
		{
			name:    "workspace servers",
			handler: h.DeleteUnauthorizedMCPServersForWorkspace,
			parent:  workspace,
			owned: &v1.MCPServer{Name: "ms1agent", Namespace: "default", Spec: v1.MCPServerSpec{
				UserID:                    "agent:7",
				MCPServerCatalogEntryName: workspaceEntry.Name,
			}},
			objects: []kclient.Object{workspaceEntry},
		},
		{
			name:    "catalog connections",
			handler: h.DeleteUnauthorizedMCPServerInstancesForCatalog,
			parent:  catalog,
			owned: &v1.MCPServerInstance{Name: "msi1agent", Namespace: "default", Spec: v1.MCPServerInstanceSpec{
				UserID:        "agent:7",
				MCPServerName: "ms1catalog",
			}},
			objects: []kclient.Object{&v1.MCPServer{Name: "ms1catalog", Namespace: "default", Spec: v1.MCPServerSpec{MCPCatalogID: catalog.Name}}},
		},
		{
			name:    "workspace connections",
			handler: h.DeleteUnauthorizedMCPServerInstancesForWorkspace,
			parent:  workspace,
			owned: &v1.MCPServerInstance{Name: "msi1agent", Namespace: "default", Spec: v1.MCPServerInstanceSpec{
				UserID:        "agent:7",
				MCPServerName: "ms1workspace",
			}},
			objects: []kclient.Object{&v1.MCPServer{Name: "ms1workspace", Namespace: "default", Spec: v1.MCPServerSpec{PowerUserWorkspaceID: workspace.Name, UserID: "1"}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newClient(append([]kclient.Object{tc.parent, tc.owned}, tc.objects...)...)
			require.NoError(t, tc.handler(router.Request{Ctx: t.Context(), Client: client, Object: tc.parent, Namespace: "default"}, nil))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(tc.owned), tc.owned.DeepCopyObject().(kclient.Object)))
		})
	}
}
