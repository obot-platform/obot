package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// agentAdminReads and agentAdminWrites are the routes of the admin API of agents, which TestAgentAdminRoutesByRole
// checks for each role.
var (
	agentAdminReads = []string{
		"/api/agents",
		"/api/agents/7",
		"/api/agents/7/api-keys",
		"/api/agents/7/api-keys/1",
	}
	agentAdminWrites = []struct {
		method string
		path   string
	}{
		{
			method: http.MethodPost,
			path:   "/api/agents",
		},
		{
			method: http.MethodPut,
			path:   "/api/agents/7",
		},
		{
			method: http.MethodDelete,
			path:   "/api/agents/7",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents/7/disable",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents/7/enable",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents/7/api-keys",
		},
		{
			method: http.MethodDelete,
			path:   "/api/agents/7/api-keys/1",
		},
	}
)

// TestAgentAdminRoutesByRole checks who reaches the admin API of agents: administrators and owners read and change
// agents and their keys, read-only administrators and auditors only read them, and no other user reaches them.
func TestAgentAdminRoutesByRole(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)

	for _, tt := range []struct {
		name   string
		role   types.Role
		reads  bool
		writes bool
	}{
		{
			name:   "owner",
			role:   types.RoleOwner,
			reads:  true,
			writes: true,
		},
		{
			name:   "administrator",
			role:   types.RoleAdmin,
			reads:  true,
			writes: true,
		},
		{
			name:  "read-only administrator",
			role:  types.RoleBasic | types.RoleAdminReadonly,
			reads: true,
		},
		{
			name:  "auditor",
			role:  types.RoleBasic | types.RoleAuditor,
			reads: true,
		},
		{
			name:  "read-only administrator and auditor",
			role:  types.RoleBasic | types.RoleAdminReadonly | types.RoleAuditor,
			reads: true,
		},
		{
			name: "power user plus",
			role: types.RolePowerUserPlus,
		},
		{
			name: "basic user",
			role: types.RoleBasic,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := &user.DefaultInfo{Name: "101", UID: "101", Groups: tt.role.Groups()}
			for _, path := range agentAdminReads {
				if got := authorizer.Authorize(httptest.NewRequest(http.MethodGet, path, nil), caller); got != tt.reads {
					t.Errorf("GET %s: authorized = %v, want %v", path, got, tt.reads)
				}
			}
			for _, route := range agentAdminWrites {
				if got := authorizer.Authorize(httptest.NewRequest(route.method, route.path, nil), caller); got != tt.writes {
					t.Errorf("%s %s: authorized = %v, want %v", route.method, route.path, got, tt.writes)
				}
			}
		})
	}
}
