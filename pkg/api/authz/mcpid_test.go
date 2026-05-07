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

func TestCheckMCPIDAllowsAnonymousMCPConnect(t *testing.T) {
	authorizer := &Authorizer{}
	req := httptest.NewRequest("GET", "/mcp-connect/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, &user.DefaultInfo{Name: "anonymous"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}

func TestCheckMCPIDDoesNotBypassNonMCPConnectForAnonymous(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest("GET", "/oauth/authorize/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, &user.DefaultInfo{Name: "anonymous"})
	if err == nil {
		t.Fatal("checkMCPID() error = nil, want error")
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksMCPServerInstanceOwner(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "msi1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID: "user-uid",
		},
	}).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest("GET", "/mcp-connect/msi1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "msi1test"}, &user.DefaultInfo{
		Name: "user",
		UID:  "user-uid",
	})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}

func TestCheckMCPIDChecksSystemMCPServerEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		allowed bool
	}{
		{
			name:    "nil enabled defaults to allowed",
			allowed: true,
		},
		{
			name:    "explicitly enabled is allowed",
			enabled: new(true),
			allowed: true,
		},
		{
			name:    "explicitly disabled is denied",
			enabled: new(false),
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.SystemMCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sms1test",
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.SystemMCPServerSpec{
					Manifest: types.SystemMCPServerManifest{
						Enabled: tt.enabled,
					},
				},
			}).Build()

			authorizer := &Authorizer{cache: storage, uncached: storage}
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/sms1test", nil)

			ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "sms1test"}, &user.DefaultInfo{
				Name: "user",
				UID:  "user-uid",
			})
			if err != nil {
				t.Fatalf("checkMCPID() error = %v", err)
			}
			if ok != tt.allowed {
				t.Fatalf("checkMCPID() = %v, want %v", ok, tt.allowed)
			}
		})
	}
}

func TestCheckMCPIDChecksMCPServerCatalogAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1catalog",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "catalog-a",
			UserID:       "owner-uid",
		},
	}).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acr1server",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "catalog-a",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServer, ID: "ms1catalog"}},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/ms1catalog", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1catalog"}, &user.DefaultInfo{Name: "user", UID: "user-uid"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}

	ok, err = authorizer.checkMCPID(req, &Resources{MCPID: "ms1catalog"}, &user.DefaultInfo{Name: "other", UID: "other-uid"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksCatalogEntryAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "entry-test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "catalog-a",
		},
	}).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acr1entry",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "catalog-a",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "entry-test"}},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/entry-test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "entry-test"}, &user.DefaultInfo{Name: "user", UID: "user-uid"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}

	ok, err = authorizer.checkMCPID(req, &Resources{MCPID: "entry-test"}, &user.DefaultInfo{Name: "other", UID: "other-uid"})
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksWorkspaceAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ms1workspace",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerSpec{
				PowerUserWorkspaceID: "puw1test",
				UserID:               "owner-uid",
			},
		},
		&v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workspace-entry",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				PowerUserWorkspaceID: "puw1test",
			},
		},
		&v1.PowerUserWorkspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "puw1test",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.PowerUserWorkspaceSpec{
				UserID: "owner-uid",
			},
		},
	).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acr1workspace",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.AccessControlRuleSpec{
			PowerUserWorkspaceID: "puw1test",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "shared-user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
			},
		},
	})

	tests := []struct {
		name    string
		mcpID   string
		userID  string
		allowed bool
	}{
		{name: "server owner is allowed", mcpID: "ms1workspace", userID: "owner-uid", allowed: true},
		{name: "server shared user is allowed", mcpID: "ms1workspace", userID: "shared-user-uid", allowed: true},
		{name: "server unrelated user is denied", mcpID: "ms1workspace", userID: "other-uid", allowed: false},
		{name: "entry workspace owner is allowed", mcpID: "workspace-entry", userID: "owner-uid", allowed: true},
		{name: "entry shared user is allowed", mcpID: "workspace-entry", userID: "shared-user-uid", allowed: true},
		{name: "entry unrelated user is denied", mcpID: "workspace-entry", userID: "other-uid", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/"+tt.mcpID, nil)

			ok, err := authorizer.checkMCPID(req, &Resources{MCPID: tt.mcpID}, &user.DefaultInfo{Name: "user", UID: tt.userID})
			if err != nil {
				t.Fatalf("checkMCPID() error = %v", err)
			}
			if ok != tt.allowed {
				t.Fatalf("checkMCPID() = %v, want %v", ok, tt.allowed)
			}
		})
	}
}

func TestMCPConnectSubtreeAuthorization(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ms1test",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerSpec{
				UserID: "user-uid",
			},
		},
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ms1keytest",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerSpec{
				UserID: "key-user-uid",
			},
		},
		&v1.MCPServerInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "msi1test",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerInstanceSpec{
				UserID: "user-uid",
			},
		},
		&v1.MCPServerInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "msi1keytest",
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerInstanceSpec{
				UserID: "key-user-uid",
			},
		},
	).Build()
	authorizer := NewAuthorizer(storage, storage, false, nil, false)

	tests := []struct {
		name    string
		method  string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name:   "basic user can access exact connect path",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user can access exact server connect path",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user can access trailing slash",
			method: http.MethodPost,
			path:   "/mcp-connect/msi1test/",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user can access server trailing slash",
			method: http.MethodPost,
			path:   "/mcp-connect/ms1test/",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user can access subpath",
			method: http.MethodDelete,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "basic user can access server subpath",
			method: http.MethodDelete,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "api key cannot access subpath for a server they don't own",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: false,
		},
		{
			name:   "api key cannot access subpath for an MCP server they don't own",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: false,
		},
		{
			name:   "api key can access subpath for a server they own",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1keytest/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: true,
		},
		{
			name:   "api key can access subpath for an MCP server they own",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1keytest/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: true,
		},
		{
			name:   "authenticated user without basic group cannot access subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "authenticated user without basic group cannot access server subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic user cannot access another user's instance subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "other-user",
				UID:    "other-user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic user cannot access another user's server subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "other-user",
				UID:    "other-user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := authorizer.Authorize(req, tt.user); got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func newMCPIDTestAuthorizer(t *testing.T, storage client.Client, acrs ...*v1.AccessControlRule) *Authorizer {
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
		"server-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeMCPServer {
					results = append(results, resource.ID)
				}
			}
			return results, nil
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

	return &Authorizer{
		cache:     storage,
		uncached:  storage,
		acrHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, storage),
	}
}
