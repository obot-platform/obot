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

	const connection = "0b6bd0a4-7e44-4c3c-9d0b-8a3d1f1b8f6e"

	tests := []struct {
		name    string
		method  string
		path    string
		uid     string
		groups  []string
		allowed bool
	}{
		{
			name:    "connection principal can list users",
			method:  http.MethodGet,
			path:    "/scim/v2/Users",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:    "connection principal can write groups",
			method:  http.MethodPatch,
			path:    "/scim/v2/Groups/abc",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:    "connection principal can reach the SCIM root, which the handler answers",
			method:  http.MethodGet,
			path:    "/scim/v2/",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:    "connection principal can reach the base URL, which the handler answers",
			method:  http.MethodGet,
			path:    "/scim/v2",
			uid:     connection,
			groups:  []string{types.GroupSCIM},
			allowed: true,
		},
		{
			name:   "connection principal cannot reach a path that only begins like the base URL",
			method: http.MethodGet,
			path:   "/scim/v2x",
			uid:    connection,
			groups: []string{types.GroupSCIM},
		},
		{
			name:   "anonymous cannot reach the base URL",
			method: http.MethodGet,
			path:   "/scim/v2",
			uid:    "anonymous",
			groups: []string{UnauthenticatedGroup},
		},
		{
			name:   "connection principal without a UID cannot reach the SCIM endpoint",
			method: http.MethodGet,
			path:   "/scim/v2/Users",
			groups: []string{types.GroupSCIM},
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
			path:   "/scim/v2/Users",
			uid:    "1",
			groups: types.RoleOwner.Groups(),
		},
		{
			name:   "anonymous cannot reach a SCIM route",
			method: http.MethodGet,
			path:   "/scim/v2/ServiceProviderConfig",
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

func TestSCIMConnectionAdministration(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)
	const connection = "0b6bd0a4-7e44-4c3c-9d0b-8a3d1f1b8f6e"

	reads := []string{
		"/api/scim-connections",
		"/api/scim-connections/enable-preview",
		"/api/scim-connections/" + connection + "/review",
		"/api/scim-connections/" + connection + "/users",
		"/api/scim-connections/" + connection + "/groups",
		"/api/scim-connections/" + connection + "/failures",
	}
	writes := []string{
		"/api/scim-connections",
		"/api/scim-connections/" + connection + "/enforce",
		"/api/scim-connections/" + connection + "/delete-unreferenced-groups",
		"/api/scim-connections/" + connection + "/rotate-token",
		"/api/scim-connections/" + connection + "/revoke-current-token",
		"/api/scim-connections/" + connection + "/revoke-previous-token",
	}

	tests := []struct {
		name        string
		groups      []string
		allowReads  bool
		allowWrites bool
		// allowResidualGroupData is whether the residual group data of an auth provider can be read.
		allowResidualGroupData bool
	}{
		{
			name:                   "owner, including the bootstrap user",
			groups:                 types.RoleOwner.Groups(),
			allowReads:             true,
			allowWrites:            true,
			allowResidualGroupData: true,
		},
		{
			name:                   "admin",
			groups:                 types.RoleAdmin.Groups(),
			allowReads:             true,
			allowResidualGroupData: true,
		},
		{
			name:       "auditor",
			groups:     (types.RoleBasic | types.RoleAuditor).Groups(),
			allowReads: true,
		},
		{
			name:   "basic user",
			groups: types.RoleBasic.Groups(),
		},
		{
			name:   "connection principal",
			groups: []string{types.GroupSCIM},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := func(method, path string, want bool) {
				t.Helper()
				req := httptest.NewRequest(method, path, nil)
				got := authorizer.Authorize(req, &user.DefaultInfo{
					Name:   "principal",
					UID:    connection,
					Groups: tt.groups,
				})
				if got != want {
					t.Errorf("Authorize(%s %s) = %v, want %v", method, path, got, want)
				}
			}
			for _, path := range reads {
				check(http.MethodGet, path, tt.allowReads)
			}
			for _, path := range writes {
				check(http.MethodPost, path, tt.allowWrites)
			}
			check(http.MethodGet, "/api/auth-providers/okta-auth-provider/residual-group-data", tt.allowResidualGroupData)
		})
	}
}
