package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLLMGatewayMigrationAuthorization(t *testing.T) {
	a := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	for _, tc := range []struct {
		name    string
		role    types.Role
		allowed bool
	}{
		{
			name:    "owner",
			role:    types.RoleOwner,
			allowed: true,
		},
		{
			name:    "admin",
			role:    types.RoleAdmin,
			allowed: true,
		},
		{
			name: "auditor",
			role: types.RoleAuditor,
		},
		{
			name: "basic",
			role: types.RoleBasic,
		},
		{
			name: "power user",
			role: types.RolePowerUser,
		},
		{
			name: "power user plus",
			role: types.RolePowerUserPlus,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &user.DefaultInfo{Name: tc.name, Groups: tc.role.Groups()}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				for _, path := range []string{"/api/llm-gateway/migration", "/api/llm-gateway/%6digration"} {
					require.Equal(t, tc.allowed, a.Authorize(httptest.NewRequest(method, path, nil), u), "%s %s", method, path)
				}
			}
		})
	}

	t.Run("auditor can still read ordinary gateway resources", func(t *testing.T) {
		u := &user.DefaultInfo{Groups: types.RoleAuditor.Groups()}
		require.True(t, a.Authorize(httptest.NewRequest(http.MethodGet, "/api/llm-gateway/providers", nil), u))
	})

	for _, group := range []string{types.GroupSCIM, types.GroupTunnel, types.GroupTunnelPeer, types.GroupTunnelBridge} {
		t.Run(group, func(t *testing.T) {
			u := &user.DefaultInfo{UID: "internal", Groups: append(types.RoleAdmin.Groups(), group)}
			require.False(t, a.Authorize(httptest.NewRequest(http.MethodGet, "/api/llm-gateway/migration", nil), u))
		})
	}
}
