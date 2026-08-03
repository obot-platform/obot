package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLicenseRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	users := []struct {
		name      string
		info      user.Info
		canRead   bool
		canMutate bool
	}{
		{
			name:      "owner",
			info:      &user.DefaultInfo{Name: "owner", Groups: types.RoleOwner.Groups()},
			canRead:   true,
			canMutate: true,
		},
		{
			name:      "administrator",
			info:      &user.DefaultInfo{Name: "admin", Groups: types.RoleAdmin.Groups()},
			canRead:   true,
			canMutate: true,
		},
		{
			name:    "auditor",
			info:    &user.DefaultInfo{Name: "auditor", Groups: types.RoleAuditor.Groups()},
			canRead: true,
		},
		{
			name:    "basic user",
			info:    &user.DefaultInfo{Name: "basic", Groups: types.RoleBasic.Groups()},
			canRead: true,
		},
		{
			name: "unauthenticated user",
			info: &user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}},
		},
	}
	routes := []struct {
		name     string
		method   string
		path     string
		readOnly bool
	}{
		{
			name:     "read status",
			method:   http.MethodGet,
			path:     "/api/license",
			readOnly: true,
		},
		{
			name:   "update key",
			method: http.MethodPut,
			path:   "/api/license",
		},
		{
			name:   "recheck key",
			method: http.MethodPost,
			path:   "/api/license",
		},
		{
			name:   "delete key",
			method: http.MethodDelete,
			path:   "/api/license",
		},
		{
			name:   "create community license",
			method: http.MethodPost,
			path:   "/api/license/community",
		},
	}

	for _, route := range routes {
		for _, testUser := range users {
			t.Run(route.name+"/"+testUser.name, func(t *testing.T) {
				request := httptest.NewRequest(route.method, route.path, nil)
				allowed := testUser.canMutate
				if route.readOnly {
					allowed = testUser.canRead
				}
				if got := authorizer.Authorize(request, testUser.info); got != allowed {
					t.Fatalf("Authorize(%s %s, %s) = %t, want %t", route.method, route.path, testUser.name, got, allowed)
				}
			})
		}
	}
}
