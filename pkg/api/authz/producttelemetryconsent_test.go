package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestProductTelemetryConsentRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	users := []struct {
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
			name:    "administrator",
			info:    &user.DefaultInfo{Name: "admin", Groups: types.RoleAdmin.Groups()},
			allowed: true,
		},
		{
			name: "auditor",
			info: &user.DefaultInfo{Name: "auditor", Groups: types.RoleAuditor.Groups()},
		},
		{
			name: "power user",
			info: &user.DefaultInfo{Name: "power-user", Groups: types.RolePowerUser.Groups()},
		},
		{
			name: "basic user",
			info: &user.DefaultInfo{Name: "basic", Groups: types.RoleBasic.Groups()},
		},
		{
			name: "unauthenticated",
			info: &user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}},
		},
	}

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		for _, testUser := range users {
			t.Run(method+"/"+testUser.name, func(t *testing.T) {
				request := httptest.NewRequest(method, "/api/product-telemetry-consent", nil)
				if got := authorizer.Authorize(request, testUser.info); got != testUser.allowed {
					t.Fatalf("Authorize(%s, %s) = %t, want %t", method, testUser.name, got, testUser.allowed)
				}
			})
		}
	}
}
