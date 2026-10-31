package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestToolListRequestRoutes(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	// The obot CLI's default scopes carry no admin groups; the handler checks the user's role.
	cli := &user.DefaultInfo{UID: "42", Groups: []string{types.GroupLLM, types.GroupSkills, types.GroupDeviceScans, types.GroupAuthenticated}}
	adminSession := &user.DefaultInfo{UID: "42", Groups: types.RoleAdmin.Groups()}
	basicSession := &user.DefaultInfo{UID: "7", Groups: types.RoleBasic.Groups()}

	tests := []struct {
		name    string
		user    user.Info
		method  string
		path    string
		allowed bool
	}{
		{
			name:    "CLI starts a request",
			user:    cli,
			method:  http.MethodPost,
			path:    "/api/tool-list-requests",
			allowed: true,
		},
		{
			name:    "CLI polls a request",
			user:    cli,
			method:  http.MethodGet,
			path:    "/api/tool-list-requests/tlr1",
			allowed: true,
		},
		{
			name:   "CLI can't answer a request",
			user:   cli,
			method: http.MethodPost,
			path:   "/api/tool-list-requests/tlr1/complete",
		},
		{
			name:   "CLI can't cancel a request",
			user:   cli,
			method: http.MethodPost,
			path:   "/api/tool-list-requests/tlr1/cancel",
		},
		{
			name:    "admin session answers a request",
			user:    adminSession,
			method:  http.MethodPost,
			path:    "/api/tool-list-requests/tlr1/complete",
			allowed: true,
		},
		{
			name:   "basic session can't answer a request",
			user:   basicSession,
			method: http.MethodPost,
			path:   "/api/tool-list-requests/tlr1/complete",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}
