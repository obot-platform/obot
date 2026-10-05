package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestEnableUserAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)

	tests := []struct {
		name    string
		groups  []string
		allowed bool
	}{
		{
			name:    "owner",
			groups:  types.RoleOwner.Groups(),
			allowed: true,
		},
		{
			name:    "admin",
			groups:  types.RoleAdmin.Groups(),
			allowed: true,
		},
		{
			name:   "auditor",
			groups: (types.RoleBasic | types.RoleAuditor).Groups(),
		},
		{
			name:   "basic user",
			groups: types.RoleBasic.Groups(),
		},
		{
			name:   "power user",
			groups: types.RolePowerUser.Groups(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/users/7/enable", nil)
			got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "principal",
				UID:    "1",
				Groups: tt.groups,
			})
			if got != tt.allowed {
				t.Fatalf("Authorize(POST /api/users/7/enable, %v) = %v, want %v", tt.groups, got, tt.allowed)
			}
		})
	}
}
