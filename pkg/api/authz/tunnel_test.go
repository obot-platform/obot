package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestTunnelAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)

	tests := []struct {
		name    string
		method  string
		path    string
		uid     string
		groups  []string
		allowed bool
	}{
		{
			name:    "tunnel principal can connect",
			method:  http.MethodGet,
			path:    "/tunnel/connect",
			uid:     "office",
			groups:  []string{types.GroupTunnel},
			allowed: true,
		},
		{
			name:   "tunnel principal cannot use old named connect path",
			method: http.MethodGet,
			path:   "/tunnel/connect/office",
			uid:    "office",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot post to connect",
			method: http.MethodPost,
			path:   "/tunnel/connect",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot head connect",
			method: http.MethodHead,
			path:   "/tunnel/connect",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot access nested connect path",
			method: http.MethodGet,
			path:   "/tunnel/connect/office/extra",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot access any-group route",
			method: http.MethodGet,
			path:   "/api/healthz",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot access UI fallthrough",
			method: http.MethodGet,
			path:   "/",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot read tunnel objects",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "tunnel principal cannot list tunnels",
			method: http.MethodGet,
			path:   "/api/tunnels",
			groups: []string{types.GroupTunnel},
		},
		{
			name:   "mixed tunnel and admin groups remain tunnel restricted",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels",
			groups: []string{types.GroupTunnel, types.GroupAdmin},
		},
		{
			name:    "tunnel peer principal can connect",
			method:  http.MethodGet,
			path:    "/tunnel/peer",
			uid:     "10.0.0.2",
			groups:  []string{types.GroupTunnelPeer},
			allowed: true,
		},
		{
			name:   "tunnel peer principal cannot post",
			method: http.MethodPost,
			path:   "/tunnel/peer",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "tunnel peer principal cannot head",
			method: http.MethodHead,
			path:   "/tunnel/peer",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "tunnel peer principal cannot access nested path",
			method: http.MethodGet,
			path:   "/tunnel/peer/extra",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "tunnel peer principal cannot connect a client tunnel",
			method: http.MethodGet,
			path:   "/tunnel/connect",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "tunnel peer principal cannot access a bridge target",
			method: http.MethodGet,
			path:   "/tunnel/bridge/target",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "tunnel peer principal cannot access any-group route",
			method: http.MethodGet,
			path:   "/api/healthz",
			groups: []string{types.GroupTunnelPeer},
		},
		{
			name:   "mixed tunnel peer and admin groups remain peer restricted",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels",
			groups: []string{types.GroupTunnelPeer, types.GroupAdmin},
		},
		{
			name:    "tunnel bridge principal can get target",
			method:  http.MethodGet,
			path:    "/tunnel/bridge/aHR0cCt0dW5uZWw6Ly9vZmZpY2VAZXhhbXBsZS50ZXN0L21jcA",
			groups:  []string{types.GroupTunnelBridge},
			allowed: true,
		},
		{
			name:    "tunnel bridge principal can post target",
			method:  http.MethodPost,
			path:    "/tunnel/bridge/aHR0cCt0dW5uZWw6Ly9vZmZpY2VAZXhhbXBsZS50ZXN0L21jcA",
			groups:  []string{types.GroupTunnelBridge},
			allowed: true,
		},
		{
			name:    "tunnel bridge principal can delete target",
			method:  http.MethodDelete,
			path:    "/tunnel/bridge/aHR0cCt0dW5uZWw6Ly9vZmZpY2VAZXhhbXBsZS50ZXN0L21jcA",
			groups:  []string{types.GroupTunnelBridge},
			allowed: true,
		},
		{
			name:   "tunnel bridge principal requires target",
			method: http.MethodGet,
			path:   "/tunnel/bridge/",
			groups: []string{types.GroupTunnelBridge},
		},
		{
			name:   "tunnel bridge principal cannot access nested target",
			method: http.MethodGet,
			path:   "/tunnel/bridge/target/extra",
			groups: []string{types.GroupTunnelBridge},
		},
		{
			name:   "tunnel bridge principal cannot connect tunnel",
			method: http.MethodGet,
			path:   "/tunnel/connect",
			groups: []string{types.GroupTunnelBridge},
		},
		{
			name:   "tunnel bridge principal cannot access any-group route",
			method: http.MethodGet,
			path:   "/api/healthz",
			groups: []string{types.GroupTunnelBridge},
		},
		{
			name:   "mixed tunnel bridge and admin groups remain bridge restricted",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels",
			groups: []string{types.GroupTunnelBridge, types.GroupAdmin},
		},
		{
			name:   "ordinary authenticated user cannot access tunnel bridge",
			method: http.MethodPost,
			path:   "/tunnel/bridge/target",
			groups: []string{types.GroupAuthenticated},
		},
		{
			name:   "ordinary authenticated user cannot connect tunnel",
			method: http.MethodGet,
			path:   "/tunnel/connect",
			groups: []string{types.GroupAuthenticated},
		},
		{
			name:   "ordinary authenticated user cannot connect a tunnel peer",
			method: http.MethodGet,
			path:   "/tunnel/peer",
			groups: []string{types.GroupAuthenticated},
		},
		{
			name:   "admin cannot connect using user authentication",
			method: http.MethodGet,
			path:   "/tunnel/connect",
			groups: types.RoleAdmin.Groups(),
		},
		{
			name:    "admin can list MCP tunnels",
			method:  http.MethodGet,
			path:    "/api/mcp-tunnels",
			groups:  types.RoleAdmin.Groups(),
			allowed: true,
		},
		{
			name:   "auditor cannot list MCP tunnels",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels",
			groups: types.RoleAuditor.Groups(),
		},
		{
			name:   "auditor cannot get an MCP tunnel",
			method: http.MethodGet,
			path:   "/api/mcp-tunnels/mt1example",
			groups: types.RoleAuditor.Groups(),
		},
		{
			name:    "admin can list tunnels",
			method:  http.MethodGet,
			path:    "/api/tunnels",
			groups:  types.RoleAdmin.Groups(),
			allowed: true,
		},
		{
			name:    "owner can list tunnels",
			method:  http.MethodGet,
			path:    "/api/tunnels",
			groups:  types.RoleOwner.Groups(),
			allowed: true,
		},
		{
			name:    "auditor can list tunnels",
			method:  http.MethodGet,
			path:    "/api/tunnels",
			groups:  types.RoleAuditor.Groups(),
			allowed: true,
		},
		{
			name:    "basic user can list tunnels",
			method:  http.MethodGet,
			path:    "/api/tunnels",
			groups:  types.RoleBasic.Groups(),
			allowed: true,
		},
		{
			name:   "admin cannot post tunnels",
			method: http.MethodPost,
			path:   "/api/tunnels",
			groups: types.RoleAdmin.Groups(),
		},
		{
			name:    "owner can rotate MCP tunnel secret",
			method:  http.MethodPost,
			path:    "/api/mcp-tunnels/mt1example/rotate-secret",
			groups:  types.RoleOwner.Groups(),
			allowed: true,
		},
		{
			name:    "admin can delete MCP tunnel",
			method:  http.MethodDelete,
			path:    "/api/mcp-tunnels/mt1example",
			groups:  types.RoleAdmin.Groups(),
			allowed: true,
		},
		{
			name:   "basic user cannot delete MCP tunnel",
			method: http.MethodDelete,
			path:   "/api/mcp-tunnels/mt1example",
			groups: types.RoleBasic.Groups(),
		},
		{
			name:   "auditor cannot delete MCP tunnel",
			method: http.MethodDelete,
			path:   "/api/mcp-tunnels/mt1example",
			groups: types.RoleAuditor.Groups(),
		},
		{
			name:    "admin can create MCP tunnel",
			method:  http.MethodPost,
			path:    "/api/mcp-tunnels",
			groups:  types.RoleAdmin.Groups(),
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			uid := tt.uid
			if uid == "" {
				uid = "test"
			}
			got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "test",
				UID:    uid,
				Groups: tt.groups,
			})
			if got != tt.allowed {
				t.Fatalf("Authorize(%s %s, %v) = %v, want %v", tt.method, tt.path, tt.groups, got, tt.allowed)
			}
		})
	}
}
