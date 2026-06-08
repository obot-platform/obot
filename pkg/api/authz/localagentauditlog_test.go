package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestLocalAgentAuditLogRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, false, nil, false)

	tests := []struct {
		name    string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name: "admin can list metadata",
			path: "/api/local-agent-audit-logs",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{types.GroupAdmin, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name: "owner can list metadata",
			path: "/api/local-agent-audit-logs",
			user: &user.DefaultInfo{
				Name:   "owner",
				Groups: []string{types.GroupOwner, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name: "auditor can get detail",
			path: "/api/local-agent-audit-logs/detail/1",
			user: &user.DefaultInfo{
				Name:   "auditor",
				Groups: []string{types.GroupAuditor, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name: "basic user cannot list",
			path: "/api/local-agent-audit-logs",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name: "basic user cannot get filter options",
			path: "/api/local-agent-audit-logs/filter-options/client_name",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name: "api key cannot query",
			path: "/api/local-agent-audit-logs",
			user: &user.DefaultInfo{
				Name:   "api-key",
				Groups: []string{types.GroupAPIKey},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}
