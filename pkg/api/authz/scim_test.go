package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestSCIMAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)

	const (
		connection = "0b6bd0a4-7e44-4c3c-9d0b-8a3d1f1b8f6e"
		other      = "9a1f4d1e-2c0b-4f3a-8e5d-6b7c8d9e0f1a"
	)

	tests := []struct {
		name    string
		method  string
		path    string
		uid     string
		groups  []string
		allowed bool
	}{
		{
			name:    "connection principal can list its users",
			method:  http.MethodGet,
			path:    "/scim/v2/" + connection + "/Users",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:    "connection principal can write its groups",
			method:  http.MethodPatch,
			path:    "/scim/v2/" + connection + "/Groups/abc",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:   "connection principal cannot reach another connection",
			method: http.MethodGet,
			path:   "/scim/v2/" + other + "/Users",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "connection principal without a UID cannot reach any connection",
			method: http.MethodGet,
			path:   "/scim/v2/" + connection + "/Users",
			groups: []string{types.GroupSCIM},
		},
		{
			name:    "connection principal can reach its base URL without a trailing slash",
			method:  http.MethodGet,
			path:    "/scim/v2/" + connection,
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:   "connection principal cannot reach another connection without a trailing slash",
			method: http.MethodGet,
			path:   "/scim/v2/" + other,
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "connection principal cannot reach the SCIM root",
			method: http.MethodGet,
			path:   "/scim/v2/",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "anonymous cannot reach a connection without a trailing slash",
			method: http.MethodGet,
			path:   "/scim/v2/" + connection,
			uid:    "anonymous",
			groups: []string{UnauthenticatedGroup},
		},
		{
			name:   "anonymous cannot reach a connection through an encoded slash",
			method: http.MethodGet,
			path:   "/scim/v2/" + connection + "%2FUsers",
			uid:    "anonymous",
			groups: []string{UnauthenticatedGroup},
		},
		{
			name:   "connection principal cannot reach an API route",
			method: http.MethodGet,
			path:   "/api/me",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "connection principal cannot reach an any-group route",
			method: http.MethodGet,
			path:   "/api/healthz",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "connection principal cannot reach the UI",
			method: http.MethodGet,
			path:   "/",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "owner cannot reach a SCIM route",
			method: http.MethodGet,
			path:   "/scim/v2/" + connection + "/Users",
			uid:    "1",
			groups: types.RoleOwner.Groups(),
		},
		{
			name:   "anonymous cannot reach a SCIM route",
			method: http.MethodGet,
			path:   "/scim/v2/" + connection + "/ServiceProviderConfig",
			uid:    "anonymous",
			groups: []string{UnauthenticatedGroup},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			// The API server's mux sets the matched pattern before authorization runs.
			if tt.path == "/" {
				req.Pattern = "/"
			} else {
				req.Pattern = "/scim/v2/"
			}
			got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "principal",
				UID:    tt.uid,
				Groups: tt.groups,
			})
			if got != tt.allowed {
				t.Fatalf("Authorize(%s %s, %v) = %v, want %v", tt.method, tt.path, tt.groups, got, tt.allowed)
			}
		})
	}
}
