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

func TestMCPGroupAllowsMCPAndAnyGroupRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "msi1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID: "mcpoauth-user-uid",
		},
	}).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, false)
	mcpUser := &user.DefaultInfo{
		Name:   "mcp-user",
		UID:    "mcpoauth-user-uid",
		Groups: []string{types.GroupMCP, types.GroupAuthenticated},
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "MCP static route",
			method: http.MethodGet,
			path:   "/oauth/userinfo",
		},
		{
			name:   "proxy anyGroup OAuth token route",
			method: http.MethodPost,
			path:   "/oauth/token",
		},
		{
			name:   "MCP connect route",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, mcpUser); !allowed {
				t.Fatalf("Authorize() = false, want true")
			}
		})
	}
}

func TestUnauthenticatedOAuthCallbackAuthorizationIsExact(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, false)
	unauthenticatedUser := &user.DefaultInfo{
		Name:   "anonymous",
		UID:    "anonymous",
		Groups: []string{UnauthenticatedGroup},
	}

	tests := []struct {
		name    string
		method  string
		path    string
		allowed bool
	}{
		{name: "exact GET callback", method: http.MethodGet, path: "/oauth/mcp/callback", allowed: true},
		{name: "callback POST", method: http.MethodPost, path: "/oauth/mcp/callback", allowed: false},
		{name: "callback subpath", method: http.MethodGet, path: "/oauth/mcp/callback/extra", allowed: false},
		{name: "protected OAuth route", method: http.MethodGet, path: "/oauth/userinfo", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := authorizer.Authorize(req, unauthenticatedUser); got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestDefaultAuthorizerAllowsMCPProxyRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			UserID: "mcp-user-uid",
		},
	}).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, false)
	mcpUser := &user.DefaultInfo{
		Name:   "mcp-user",
		UID:    "mcp-user-uid",
		Groups: []string{types.GroupMCP, types.GroupAuthenticated},
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "MCP connect route",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test",
		},
		{
			name:   "MCP OAuth route",
			method: http.MethodPost,
			path:   "/oauth/token",
		},
		{
			name:   "well-known route",
			method: http.MethodGet,
			path:   "/.well-known/oauth-protected-resource",
		},
		{
			name:   "MCP OAuth userinfo route",
			method: http.MethodGet,
			path:   "/oauth/userinfo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, mcpUser); !allowed {
				t.Fatalf("Authorize() = false, want true")
			}
		})
	}
}

func TestMCPGroupDeniesNonMCPAPIRoutes(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, false)
	mcpUser := &user.DefaultInfo{
		Name:   "mcp-user",
		UID:    "mcp-user-uid",
		Groups: []string{types.GroupMCP, types.GroupAuthenticated},
	}

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "basic route is denied",
			method: http.MethodGet,
			path:   "/api/models",
		},
		{
			name:   "admin route is denied",
			method: http.MethodPatch,
			path:   "/api/model-providers/provider-id",
		},
		{
			name:   "skills route is denied",
			method: http.MethodGet,
			path:   "/api/skills",
		},
		{
			name:   "metrics route is denied",
			method: http.MethodGet,
			path:   "/debug/metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, mcpUser); allowed {
				t.Fatalf("Authorize() = true, want false")
			}
		})
	}
}

func TestAPIGroupAllowsOwnedMCPServerInstanceOAuthRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "instance-1",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID: "owner-uid",
		},
	}).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, false)
	apiUser := &user.DefaultInfo{
		Name:   "owner",
		UID:    "owner-uid",
		Groups: []string{types.GroupAPI, types.GroupAuthenticated},
	}

	tests := []struct {
		name string
		path string
	}{
		{
			name: "oauth url",
			path: "/api/mcp-server-instances/instance-1/oauth-url",
		},
		{
			name: "oauth redirect",
			path: "/api/mcp-server-instances/instance-1/oauth-redirect",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if allowed := authorizer.Authorize(req, apiUser); !allowed {
				t.Fatalf("Authorize() = false, want true")
			}
		})
	}
}
