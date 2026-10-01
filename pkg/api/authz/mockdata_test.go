package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestMockDataRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	tests := []struct {
		name    string
		info    user.Info
		allowed bool
	}{
		{
			name:    "owner",
			info:    &user.DefaultInfo{Name: "owner", Groups: types.RoleOwner.Groups()},
			allowed: true,
		},
		{
			name: "administrator",
			info: &user.DefaultInfo{Name: "admin", Groups: types.RoleAdmin.Groups()},
		},
		{
			name: "auditor",
			info: &user.DefaultInfo{Name: "auditor", Groups: types.RoleAuditor.Groups()},
		},
		{
			name: "basic user",
			info: &user.DefaultInfo{Name: "basic", Groups: types.RoleBasic.Groups()},
		},
		{
			name: "unauthenticated user",
			info: &user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/mock-data", nil)
			if got := authorizer.Authorize(request, tt.info); got != tt.allowed {
				t.Fatalf("Authorize(POST /api/mock-data, %s) = %t, want %t", tt.name, got, tt.allowed)
			}
		})
	}
}
