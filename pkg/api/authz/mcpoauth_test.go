package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMCPGroupAllowsMCPAndAnyGroupRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{
			UserID:   "mcpoauth-user-uid",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "component"}}},
		}},
		&v1.VMCPInstance{Name: "vmcpi1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
			UserID:   "mcpoauth-user-uid",
			Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"},
		}},
	).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
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
			path:   "/mcp-connect/vmcpi1test",
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

func TestDefaultAuthorizerAllowsMCPProxyRoutes(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.VMCP{
		Name:      "vmcp1test",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			UserID:   "mcp-user-uid",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "component"}}},
		},
	}).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
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
			path:   "/mcp-connect/vmcp1test",
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
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
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

func TestMCPScopedComponentOAuth(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.VMCP{
			Name:      "vmcp1allowed",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{
				UserID:   "7",
				Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "gmail"}}},
			},
		},
		&v1.VMCPInstance{
			Name:      "vmcpi1allowed",
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPInstanceSpec{
				UserID:   "7",
				Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1allowed"},
			},
		},
	).Build()
	a := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
	runner := []string{types.GroupMCP, types.GroupClaudeEnvironment, types.GroupAuthenticated}
	for _, tc := range []struct {
		name    string
		method  string
		path    string
		userID  string
		scope   string
		groups  []string
		allowed bool
	}{
		{
			name:    "pending URLs and completion status",
			method:  http.MethodGet,
			path:    "/api/oauth/vmcp/vmcp1allowed",
			userID:  "7",
			scope:   "vmcp1allowed",
			allowed: true,
		},
		{
			name:   "MCP key outside a Claude environment",
			method: http.MethodGet,
			path:   "/api/oauth/vmcp/vmcp1allowed",
			userID: "7",
			scope:  "vmcp1allowed",
			groups: []string{types.GroupMCP, types.GroupAuthenticated},
		},
		{
			name:    "own instance",
			method:  http.MethodGet,
			path:    "/api/oauth/vmcp/vmcpi1allowed",
			userID:  "7",
			scope:   "vmcp1allowed",
			allowed: true,
		},
		{
			name:   "outside key scope",
			method: http.MethodGet,
			path:   "/api/oauth/vmcp/vmcp1allowed",
			userID: "7",
			scope:  "vmcp1other",
		},
		{
			name:   "another user's connection",
			method: http.MethodGet,
			path:   "/api/oauth/vmcp/vmcpi1allowed",
			userID: "8",
			scope:  "vmcp1allowed",
		},
		{
			name:   "no current access",
			method: http.MethodGet,
			path:   "/api/oauth/vmcp/vmcp1allowed",
			userID: "8",
			scope:  "vmcp1allowed",
		},
		{
			name:   "cannot reset credentials",
			method: http.MethodDelete,
			path:   "/api/vmcps/vmcp1allowed/oauth",
			userID: "7",
			scope:  "vmcp1allowed",
		},
		{
			name:   "cannot manage vMCP",
			method: http.MethodPut,
			path:   "/api/vmcps/vmcp1allowed",
			userID: "7",
			scope:  "vmcp1allowed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groups := tc.groups
			if groups == nil {
				groups = runner
			}
			caller := &user.DefaultInfo{
				Name:   "runner",
				UID:    tc.userID,
				Groups: groups,
				Extra:  map[string][]string{"authorized_mcp_ids": {tc.scope}},
			}
			if got := a.Authorize(httptest.NewRequest(tc.method, tc.path, nil), caller); got != tc.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tc.allowed)
			}
		})
	}
}
