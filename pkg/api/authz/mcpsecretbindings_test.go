package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestMCPSecretBindingRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, false)

	tests := []struct {
		name    string
		user    user.Info
		allowed bool
	}{
		{
			name: "admin can list allowed secret bindings",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: types.RoleAdmin.Groups(),
			},
			allowed: true,
		},
		{
			name: "owner can list allowed secret bindings",
			user: &user.DefaultInfo{
				Name:   "owner",
				Groups: types.RoleOwner.Groups(),
			},
			allowed: true,
		},
		{
			name: "auditor cannot list allowed secret bindings",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: types.RoleAuditor.Groups(),
			},
			allowed: false,
		},
		{
			name: "basic user cannot list allowed secret bindings",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: types.RoleBasic.Groups(),
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/mcp-server-binding-secrets", nil)
			assert.Equal(t, tt.allowed, authorizer.Authorize(req, tt.user))
		})
	}
}
