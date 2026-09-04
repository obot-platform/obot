package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestMDMRouteAuthorization checks which principals can read and which can
// mutate the MDM configuration and asset routes.
func TestMDMRouteAuthorization(t *testing.T) {
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
			name:    "basic user with auditor role",
			info:    &user.DefaultInfo{Name: "basic-auditor", Groups: (types.RoleBasic | types.RoleAuditor).Groups()},
			canRead: true,
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
			name: "device enrollment token",
			info: &user.DefaultInfo{Name: "enroll", Groups: []string{types.GroupDeviceEnroll}},
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
			name:     "list configurations",
			method:   http.MethodGet,
			path:     "/api/mdm/configurations",
			readOnly: true,
		},
		{
			name:     "get configuration",
			method:   http.MethodGet,
			path:     "/api/mdm/configurations/1",
			readOnly: true,
		},
		{
			name:     "list enrollment keys",
			method:   http.MethodGet,
			path:     "/api/mdm/configurations/1/enrollment-keys",
			readOnly: true,
		},
		{
			name:     "list enrolled devices",
			method:   http.MethodGet,
			path:     "/api/mdm/configurations/1/devices",
			readOnly: true,
		},
		{
			name:     "download rendered artifact",
			method:   http.MethodGet,
			path:     "/api/mdm/configurations/1/download/macos-jamf",
			readOnly: true,
		},
		{
			name:     "get asset source",
			method:   http.MethodGet,
			path:     "/api/mdm/asset-source",
			readOnly: true,
		},
		{
			name:     "list assets",
			method:   http.MethodGet,
			path:     "/api/mdm/assets",
			readOnly: true,
		},
		{
			name:   "create configuration",
			method: http.MethodPost,
			path:   "/api/mdm/configurations",
		},
		{
			name:   "update configuration",
			method: http.MethodPut,
			path:   "/api/mdm/configurations/1",
		},
		{
			name:   "update enforcement",
			method: http.MethodPut,
			path:   "/api/mdm/configurations/1/enforcement",
		},
		{
			name:   "delete configuration",
			method: http.MethodDelete,
			path:   "/api/mdm/configurations/1",
		},
		{
			name:   "create enrollment key",
			method: http.MethodPost,
			path:   "/api/mdm/configurations/1/enrollment-keys",
		},
		{
			name:   "delete enrollment key",
			method: http.MethodDelete,
			path:   "/api/mdm/configurations/1/enrollment-keys/2",
		},
		{
			name:   "refresh asset source",
			method: http.MethodPost,
			path:   "/api/mdm/asset-source/refresh",
		},
	}

	for _, route := range routes {
		for _, testUser := range users {
			t.Run(route.name+"/"+testUser.name, func(t *testing.T) {
				want := testUser.canMutate
				if route.readOnly {
					want = testUser.canRead
				}
				request := httptest.NewRequest(route.method, route.path, nil)
				if got := authorizer.Authorize(request, testUser.info); got != want {
					t.Fatalf("Authorize(%s %s, %s) = %t, want %t", route.method, route.path, testUser.name, got, want)
				}
			})
		}
	}
}

// TestMDMEnrollRouteAuthorization checks that only enrollment tokens can
// reach the device enrollment route.
func TestMDMEnrollRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	users := []struct {
		name    string
		info    user.Info
		allowed bool
	}{
		{
			name:    "device enrollment token",
			info:    &user.DefaultInfo{Name: "enroll", Groups: []string{types.GroupDeviceEnroll}},
			allowed: true,
		},
		{
			name: "owner",
			info: &user.DefaultInfo{Name: "owner", Groups: types.RoleOwner.Groups()},
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

	for _, testUser := range users {
		t.Run(testUser.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/mdm/enroll", nil)
			if got := authorizer.Authorize(request, testUser.info); got != testUser.allowed {
				t.Fatalf("Authorize(POST /api/mdm/enroll, %s) = %t, want %t", testUser.name, got, testUser.allowed)
			}
		})
	}
}
