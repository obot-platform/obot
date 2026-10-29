package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestProvenanceRouteAuthorization checks that administrators and owners can
// read and change the provenance API, auditors can only read it, and nobody
// else reaches it.
func TestProvenanceRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
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
			name: "power user plus",
			info: &user.DefaultInfo{Name: "power-user-plus", Groups: types.RolePowerUserPlus.Groups()},
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
			name: "api key without a role",
			info: &user.DefaultInfo{Name: "api", Groups: []string{types.GroupAuthenticated, types.GroupAPI}},
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
			name:     "overview",
			method:   http.MethodGet,
			path:     "/api/provenance/overview",
			readOnly: true,
		},
		{
			name:     "list sessions",
			method:   http.MethodGet,
			path:     "/api/provenance/sessions",
			readOnly: true,
		},
		{
			name:     "session events",
			method:   http.MethodGet,
			path:     "/api/provenance/sessions/ses-1/events",
			readOnly: true,
		},
		{
			name:     "session narrative",
			method:   http.MethodGet,
			path:     "/api/provenance/sessions/ses-1/narrative",
			readOnly: true,
		},
		{
			name:     "decisions feed",
			method:   http.MethodGet,
			path:     "/api/provenance/events",
			readOnly: true,
		},
		{
			name:     "list policies",
			method:   http.MethodGet,
			path:     "/api/provenance/policies",
			readOnly: true,
		},
		{
			name:     "list approvals",
			method:   http.MethodGet,
			path:     "/api/provenance/approvals",
			readOnly: true,
		},
		{
			name:     "get settings",
			method:   http.MethodGet,
			path:     "/api/provenance/settings",
			readOnly: true,
		},
		{
			name:   "issue warrant",
			method: http.MethodPost,
			path:   "/api/provenance/sessions/ses-1/warrant",
		},
		{
			name:   "end session",
			method: http.MethodPost,
			path:   "/api/provenance/sessions/ses-1/end",
		},
		{
			name:   "create policy",
			method: http.MethodPost,
			path:   "/api/provenance/policies",
		},
		{
			name:   "validate policy",
			method: http.MethodPost,
			path:   "/api/provenance/policies/validate",
		},
		{
			name:   "update policy",
			method: http.MethodPut,
			path:   "/api/provenance/policies/pol-1",
		},
		{
			name:   "delete policy",
			method: http.MethodDelete,
			path:   "/api/provenance/policies/pol-1",
		},
		{
			name:   "approve",
			method: http.MethodPost,
			path:   "/api/provenance/approvals/apr-1/approve",
		},
		{
			name:   "kill",
			method: http.MethodPost,
			path:   "/api/provenance/kills",
		},
		{
			name:   "unkill",
			method: http.MethodDelete,
			path:   "/api/provenance/kills/kil-1",
		},
		{
			name:   "plant honeytoken",
			method: http.MethodPost,
			path:   "/api/provenance/honeytokens",
		},
		{
			name:   "label tool",
			method: http.MethodPut,
			path:   "/api/provenance/tool-labels",
		},
		{
			name:   "update settings",
			method: http.MethodPut,
			path:   "/api/provenance/settings",
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

// TestCopilotStudioRouteAuthorization checks that the Copilot Studio runtime
// monitoring hook is reachable by administrators and by API keys, and by no
// one else.
func TestCopilotStudioRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	users := []struct {
		name    string
		info    user.Info
		allowed bool
	}{
		{
			name:    "owner",
			info:    &user.DefaultInfo{Name: "owner", Groups: types.RoleOwner.RoleGroups()},
			allowed: true,
		},
		{
			name:    "administrator",
			info:    &user.DefaultInfo{Name: "admin", Groups: types.RoleAdmin.RoleGroups()},
			allowed: true,
		},
		{
			name:    "api key",
			info:    &user.DefaultInfo{Name: "api", Groups: []string{types.GroupAuthenticated, types.GroupAPI}},
			allowed: true,
		},
		{
			name:    "basic user with the api group",
			info:    &user.DefaultInfo{Name: "basic-api", Groups: types.RoleBasic.Groups()},
			allowed: true,
		},
		{
			name: "basic user without the api group",
			info: &user.DefaultInfo{Name: "basic", Groups: types.RoleBasic.RoleGroups()},
		},
		{
			name: "auditor without the api group",
			info: &user.DefaultInfo{Name: "auditor", Groups: types.RoleAuditor.RoleGroups()},
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
	for _, testUser := range users {
		t.Run(testUser.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/copilot-studio/runtime-monitor", nil)
			if got := authorizer.Authorize(request, testUser.info); got != testUser.allowed {
				t.Fatalf("Authorize(POST /api/copilot-studio/runtime-monitor, %s) = %t, want %t", testUser.name, got, testUser.allowed)
			}
			// Only the hook itself is exposed, not the rest of the prefix.
			other := httptest.NewRequest(http.MethodGet, "/api/copilot-studio/runtime-monitor", nil)
			if got := authorizer.Authorize(other, testUser.info); got {
				t.Fatalf("GET on the hook must not be authorized for %s", testUser.name)
			}
		})
	}
}
