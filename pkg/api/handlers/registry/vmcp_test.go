package registry

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type registryTestStorage struct{ kclient.WithWatch }

func TestVMCPComponentsAreExcludedFromPersonalRegistryServers(t *testing.T) {
	storage := newRegistryTestStorage(
		registryOrdinaryServer("ms1ordinary", v1.MCPServerSpec{UserID: "user-1"}),
		registryVMCPComponent("ms1shared", true, v1.MCPServerSpec{UserID: "user-1"}),
		registryVMCPComponent("ms1dedicated", false, v1.MCPServerSpec{UserID: "user-1"}),
	)

	_, ctx := registryTestHandlerAndContext(t, storage)
	servers, _, err := NewHandler(nil, "https://obot.example.com", false, "").listPersonalServers(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "ms1ordinary" {
		t.Fatalf("listPersonalServers() = %#v, want only ordinary server", servers)
	}
}

func TestVMCPComponentsAreExcludedFromCatalogRegistryServers(t *testing.T) {
	storage := newRegistryTestStorage(
		registryOrdinaryServer("ms1ordinary", v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
		registryVMCPComponent("ms1shared", true, v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
		registryVMCPComponent("ms1dedicated", false, v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
	)

	h, ctx := registryTestHandlerAndContext(t, storage)
	servers, _, err := h.listServersInCatalog(ctx, system.DefaultCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "ms1ordinary" {
		t.Fatalf("listServersInCatalog() = %#v, want only ordinary server", servers)
	}
}

func TestVMCPComponentsAreExcludedFromWorkspaceRegistryServers(t *testing.T) {
	const workspaceID = "puw1workspace"
	storage := newRegistryTestStorage(
		&v1.PowerUserWorkspace{ObjectMeta: registryObjectMeta(workspaceID)},
		registryOrdinaryServer("ms1ordinary", v1.MCPServerSpec{PowerUserWorkspaceID: workspaceID, UserID: "user-1"}),
		registryVMCPComponent("ms1shared", true, v1.MCPServerSpec{PowerUserWorkspaceID: workspaceID, UserID: "user-1"}),
		registryVMCPComponent("ms1dedicated", false, v1.MCPServerSpec{PowerUserWorkspaceID: workspaceID, UserID: "user-1"}),
	)

	_, ctx := registryTestHandlerAndContext(t, storage)
	servers, _, err := NewHandler(nil, "https://obot.example.com", false, "").listServersInWorkspaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "ms1ordinary" {
		t.Fatalf("listServersInWorkspaces() = %#v, want only ordinary server", servers)
	}
}

func TestVMCPComponentsAreExcludedFromUnauthenticatedRegistryServers(t *testing.T) {
	storage := newRegistryTestStorage(
		registryOrdinaryServer("ms1ordinary", v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
		registryVMCPComponent("ms1shared", true, v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
		registryVMCPComponent("ms1dedicated", false, v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog}),
	)

	h, ctx := registryTestHandlerAndContext(t, storage)
	servers, err := h.collectAccessibleServersNoAuth(ctx, "com.example.obot")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Server.Name != "com.example.obot/ms1ordinary" {
		t.Fatalf("collectAccessibleServersNoAuth() = %#v, want only ordinary server", servers)
	}
}

func TestRegistryDirectLookupExcludesVMCPComponents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{
			name:   "shared",
			shared: true,
		},
		{
			name: "dedicated",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := registryVMCPComponent("ms1"+tc.name, tc.shared, v1.MCPServerSpec{MCPCatalogID: system.DefaultCatalog})
			h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server))
			_, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
			if err == nil {
				t.Fatal("findMCPServer() found a vMCP component")
			}
		})
	}
}

func TestRegistryDirectLookupIncludesOrdinaryServer(t *testing.T) {
	server := registryOrdinaryServer("ms1ordinary", v1.MCPServerSpec{UserID: "user-1"})
	h, ctx := registryTestHandlerAndContext(t, newRegistryTestStorage(server))

	got, err := h.findMCPServer(ctx, server.Name, "com.example.obot")
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.Name != "com.example.obot/ms1ordinary" {
		t.Fatalf("findMCPServer() returned %q, want ordinary server", got.Server.Name)
	}
}

func registryTestHandlerAndContext(t *testing.T, storage *registryTestStorage) (*Handler, api.Context) {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	client := gateway.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = client.Close() })

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"selectors": func(any) ([]string, error) { return []string{"*"}, nil },
	})
	if err := indexer.Add(&v1.AccessControlRule{
		ObjectMeta: registryObjectMeta("acr1allow"),
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest:     types.AccessControlRuleManifest{Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return NewHandler(accesscontrolrule.NewAccessControlRuleHelper(indexer, storage), "https://obot.example.com", false, ""), registryTestContext(storage, client)
}

func registryTestContext(storage *registryTestStorage, gatewayClient *gateway.Client) api.Context {
	return api.Context{
		Request:       httptest.NewRequest(http.MethodGet, "/v0.1/servers", nil),
		Storage:       storage,
		GatewayClient: gatewayClient,
		User:          &user.DefaultInfo{UID: "user-1"},
	}
}

func newRegistryTestStorage(objects ...kclient.Object) *registryTestStorage {
	return &registryTestStorage{WithWatch: clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(object kclient.Object) []string {
			return []string{object.(*v1.MCPServerCatalogEntry).Spec.MCPCatalogName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(object kclient.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.UserID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.mcpCatalogID", func(object kclient.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.MCPCatalogID}
		}).
		WithIndex(&v1.MCPServer{}, "spec.powerUserWorkspaceID", func(object kclient.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.PowerUserWorkspaceID}
		}).
		Build()}
}

func registryVMCPComponent(name string, shared bool, spec v1.MCPServerSpec) *v1.MCPServer {
	spec.VMCPComponentID = "component-1"
	if shared {
		spec.VMCPID = "vmcp1shared"
	} else {
		spec.VMCPInstanceID = "vmcpi1dedicated"
	}
	return &v1.MCPServer{ObjectMeta: registryObjectMeta(name), Spec: spec}
}

func registryOrdinaryServer(name string, spec v1.MCPServerSpec) *v1.MCPServer {
	return &v1.MCPServer{ObjectMeta: registryObjectMeta(name), Spec: spec}
}

func registryObjectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
}
