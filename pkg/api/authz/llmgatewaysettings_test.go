package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

// Every signed-in user may read whether Claude subscriptions are allowed, for the gateway's
// setup instructions; only administrators may change it.
func TestLLMGatewaySettingsAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, false)
	tests := []struct {
		name  string
		role  types.Role
		write bool
	}{
		{
			name:  "owner",
			role:  types.RoleOwner,
			write: true,
		},
		{
			name:  "admin",
			role:  types.RoleAdmin,
			write: true,
		},
		{
			name: "auditor",
			role: types.RoleAuditor,
		},
		{
			name: "basic user",
			role: types.RoleBasic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &user.DefaultInfo{Name: tt.name, Groups: tt.role.Groups()}
			if !authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/llm-gateway/settings", nil), info) {
				t.Error("GET refused, want it allowed")
			}
			if got := authorizer.Authorize(httptest.NewRequest(http.MethodPut, "/api/llm-gateway/settings", nil), info); got != tt.write {
				t.Errorf("PUT allowed = %t, want %t", got, tt.write)
			}
			if tt.role == types.RoleBasic && authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/llm-gateway/budgets", nil), info) {
				t.Error("a basic user may read other gateway APIs")
			}
		})
	}
}
