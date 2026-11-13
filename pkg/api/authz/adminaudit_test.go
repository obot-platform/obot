package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestAdminAuditEventsAuthorization is the matrix of the admin change audit's read API: administrators, owners,
// read-only administrators and auditors read it, nobody else does, and nobody, whatever their role, changes it.
func TestAdminAuditEventsAuthorization(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)

	for _, tt := range []struct {
		name   string
		user   user.Info
		method string
		want   bool
	}{
		{
			name:   "admin reads",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleAdmin.Groups()},
			method: http.MethodGet,
			want:   true,
		},
		{
			name:   "owner reads",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleOwner.Groups()},
			method: http.MethodGet,
			want:   true,
		},
		{
			name:   "auditor reads",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleAuditor.Groups()},
			method: http.MethodGet,
			want:   true,
		},
		{
			name:   "read-only admin reads",
			user:   &user.DefaultInfo{UID: "7", Groups: (types.RoleBasic | types.RoleAdminReadonly).Groups()},
			method: http.MethodGet,
			want:   true,
		},
		{
			name:   "power user plus is refused",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RolePowerUserPlus.Groups()},
			method: http.MethodGet,
		},
		{
			name:   "power user is refused",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RolePowerUser.Groups()},
			method: http.MethodGet,
		},
		{
			name:   "basic user is refused",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleBasic.Groups()},
			method: http.MethodGet,
		},
		{
			name:   "unauthenticated caller is refused",
			user:   &user.DefaultInfo{Name: "anonymous", Groups: []string{"system:unauthenticated"}},
			method: http.MethodGet,
		},
		{
			name:   "agent with every scope is refused",
			user:   gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupLLM, types.GroupSkills, types.GroupMCP, types.GroupAuthenticated}, map[string][]string{"authorized_mcp_ids": {"*"}}),
			method: http.MethodGet,
		},
		{
			name:   "agent with an administrator's groups is refused",
			user:   gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, types.RoleAdmin.Groups(), map[string][]string{"authorized_mcp_ids": {"*"}}),
			method: http.MethodGet,
		},
		{
			name:   "admin cannot write",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleAdmin.Groups()},
			method: http.MethodPost,
		},
		{
			name:   "owner cannot delete",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleOwner.Groups()},
			method: http.MethodDelete,
		},
		{
			name:   "auditor cannot write",
			user:   &user.DefaultInfo{UID: "7", Groups: types.RoleAuditor.Groups()},
			method: http.MethodPut,
		},
		{
			name:   "read-only admin cannot write",
			user:   &user.DefaultInfo{UID: "7", Groups: (types.RoleBasic | types.RoleAdminReadonly).Groups()},
			method: http.MethodPost,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/admin-audit-events?resourceKind=agent", nil)
			if got := authorizer.Authorize(req, tt.user); got != tt.want {
				t.Errorf("Authorize = %v, want %v", got, tt.want)
			}
		})
	}
}
