package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAllMCPCatalogEntryAuthorizationUsesAccessControlRules(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "entry-test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
		},
	}).Build()
	authorizer := newCatalogEntryTestAuthorizer(t, storage, &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "entry-access",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed-user"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "entry-test"}},
			},
		},
	})

	tests := []struct {
		name    string
		userID  string
		allowed bool
	}{
		{name: "user with entry ACR is allowed", userID: "allowed-user", allowed: true},
		{name: "user without entry ACR is denied", userID: "other-user", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/all-mcps/entries/entry-test", nil)
			u := &user.DefaultInfo{
				Name:   tt.userID,
				UID:    tt.userID,
				Groups: []string{types.GroupAPI, types.GroupAuthenticated},
			}

			if allowed := authorizer.Authorize(req, u); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}

func TestPowerUserWorkspaceOAuthCredentialRoutesResolveOwnedEntry(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.PowerUserWorkspace{
			ObjectMeta: metav1.ObjectMeta{Name: "workspace-1", Namespace: system.DefaultNamespace},
			Spec:       v1.PowerUserWorkspaceSpec{UserID: "owner-user"},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{Name: "entry-1", Namespace: system.DefaultNamespace},
			Spec:       v1.MCPServerCatalogEntrySpec{PowerUserWorkspaceID: "workspace-1"},
		},
	).Build()
	authorizer := newCatalogEntryTestAuthorizer(t, storage)

	routes := []struct {
		name   string
		method string
		path   string
	}{
		{name: "start test", method: http.MethodPost, path: "/api/workspaces/workspace-1/entries/entry-1/oauth-credential-tests"},
		{name: "read status", method: http.MethodPost, path: "/api/workspaces/workspace-1/entries/entry-1/oauth-credential-tests/status"},
	}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			vars, matched := authorizer.apiResources[types.GroupPowerUser].Match(req)
			if !matched {
				t.Fatal("Power User OAuth credential test route is not authorized")
			}
			if vars("workspace_id") != "workspace-1" || vars("entry_id") != "entry-1" {
				t.Fatalf("route variables = workspace %q, entry %q", vars("workspace_id"), vars("entry_id"))
			}

			owner := &user.DefaultInfo{
				Name:   "owner-user",
				UID:    "owner-user",
				Groups: []string{types.GroupPowerUser, types.GroupAuthenticated},
			}
			if !authorizer.Authorize(req, owner) {
				t.Fatal("Power User owner was denied access to their workspace entry")
			}
		})
	}
}

func TestWorkspaceOAuthCredentialTestRoutesRejectLowerRoleAndForeignOwner(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.PowerUserWorkspace{
			ObjectMeta: metav1.ObjectMeta{Name: "workspace-1", Namespace: system.DefaultNamespace},
			Spec:       v1.PowerUserWorkspaceSpec{UserID: "owner-user"},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{Name: "entry-1", Namespace: system.DefaultNamespace},
			Spec:       v1.MCPServerCatalogEntrySpec{PowerUserWorkspaceID: "workspace-1"},
		},
	).Build()
	authorizer := newCatalogEntryTestAuthorizer(t, storage)

	requests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "start test", method: http.MethodPost, path: "/api/workspaces/workspace-1/entries/entry-1/oauth-credential-tests"},
		{name: "read status", method: http.MethodPost, path: "/api/workspaces/workspace-1/entries/entry-1/oauth-credential-tests/status"},
	}
	users := []struct {
		name string
		user *user.DefaultInfo
	}{
		{
			name: "basic workspace owner",
			user: &user.DefaultInfo{Name: "owner-user", UID: "owner-user", Groups: []string{types.GroupBasic, types.GroupAuthenticated}},
		},
		{
			name: "foreign power user",
			user: &user.DefaultInfo{Name: "other-user", UID: "other-user", Groups: []string{types.GroupPowerUser, types.GroupAuthenticated}},
		},
	}

	for _, request := range requests {
		for _, tt := range users {
			t.Run(request.name+"/"+tt.name, func(t *testing.T) {
				req := httptest.NewRequest(request.method, request.path, nil)
				if authorizer.Authorize(req, tt.user) {
					t.Fatal("unauthorized user was allowed to use workspace OAuth credential test route")
				}
			})
		}
	}
}

func TestDefaultCatalogOAuthCredentialMutationsRequireOwnerGroup(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := newCatalogEntryTestAuthorizer(t, storage)
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credential-tests"},
		{method: http.MethodPost, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credential-tests/status"},
		{method: http.MethodGet, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credentials"},
		{method: http.MethodPost, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credentials"},
		{method: http.MethodPut, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credentials"},
		{method: http.MethodDelete, path: "/api/mcp-catalogs/default/entries/entry-1/oauth-credentials"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			admin := &user.DefaultInfo{Name: "admin", UID: "admin", Groups: []string{types.GroupAdmin, types.GroupAuthenticated}}
			if authorizer.Authorize(req, admin) {
				t.Fatal("admin bypassed owner-only catalog OAuth boundary")
			}
			owner := &user.DefaultInfo{Name: "owner", UID: "owner", Groups: []string{types.GroupOwner, types.GroupAdmin, types.GroupAuthenticated}}
			if !authorizer.Authorize(req, owner) {
				t.Fatal("owner was denied catalog OAuth management")
			}
		})
	}
}

func newCatalogEntryTestAuthorizer(t *testing.T, storage client.Client, acrs ...*v1.AccessControlRule) *Authorizer {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeMCPServerCatalogEntry {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"server-names": func(any) ([]string, error) {
			return nil, nil
		},
		"selectors": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	})

	for _, acr := range acrs {
		if err := indexer.Add(acr); err != nil {
			t.Fatalf("add access control rule to indexer: %v", err)
		}
	}

	return NewAuthorizer(nil, storage, storage, false, accesscontrolrule.NewAccessControlRuleHelper(indexer, storage), nil, false)
}
