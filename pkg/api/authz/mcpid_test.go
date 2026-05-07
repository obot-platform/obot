package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
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
