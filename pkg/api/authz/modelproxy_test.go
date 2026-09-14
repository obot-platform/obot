package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestModelProxyRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	users := []struct {
		name  string
		role  types.Role
		read  bool
		write bool
	}{
		{
			name:  "owner",
			role:  types.RoleOwner,
			read:  true,
			write: true,
		},
		{
			name:  "admin",
			role:  types.RoleAdmin,
			read:  true,
			write: true,
		},
		{
			name: "auditor",
			role: types.RoleAuditor,
			read: true,
		},
		{
			name: "power user",
			role: types.RolePowerUser,
		},
		{
			name: "basic user",
			role: types.RoleBasic,
		},
	}
	routes := []struct {
		method string
		path   string
	}{
		{
			method: http.MethodGet,
			path:   "/api/model-proxy",
		},
		{
			method: http.MethodGet,
			path:   "/api/model-proxy/usage",
		},
		{
			method: http.MethodPut,
			path:   "/api/model-proxy",
		},
	}
	for _, u := range users {
		for _, route := range routes {
			t.Run(u.name+"/"+route.method+route.path, func(t *testing.T) {
				info := &user.DefaultInfo{Name: u.name, Groups: u.role.Groups()}
				want := u.write
				if route.method == http.MethodGet {
					want = u.read
				}
				request := httptest.NewRequest(route.method, route.path, nil)
				if got := authorizer.Authorize(request, info); got != want {
					t.Fatalf("Authorize = %t, want %t", got, want)
				}
			})
		}
	}
}
