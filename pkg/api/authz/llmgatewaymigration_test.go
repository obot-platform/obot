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
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &user.DefaultInfo{Name: tc.name, Groups: tc.role.Groups()}
			require.Equal(t, tc.allowed, a.Authorize(httptest.NewRequest(http.MethodGet, "/api/llm-gateway/migration", nil), u))
		})
	}
}
