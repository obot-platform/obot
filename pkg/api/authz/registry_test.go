package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestRegistryRouteAuthorization(t *testing.T) {
	tests := []struct {
		name           string
		registryNoAuth bool
		user           user.Info
		allowed        bool
	}{
		{
			name:           "auth mode rejects unauthenticated user",
			registryNoAuth: false,
			user: &user.DefaultInfo{
				Name:   "anonymous",
				UID:    "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			allowed: false,
		},
		{
			name:           "auth mode allows basic user",
			registryNoAuth: false,
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:           "auth mode allows MCP OAuth user",
			registryNoAuth: false,
			user: &user.DefaultInfo{
				UID:    "user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: true,
		},
		{
			name:           "no-auth mode allows unauthenticated user",
			registryNoAuth: true,
			user: &user.DefaultInfo{
				Name:   "anonymous",
				UID:    "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, tt.registryNoAuth)
			for _, path := range []string{
				"/v0.1/servers",
				"/v0.1/servers/com.example%2Fvmcp1example/versions",
				"/v0.1/servers/com.example%2Fvmcp1example/versions/latest",
			} {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
					t.Fatalf("Authorize(%s) = %v, want %v", path, allowed, tt.allowed)
				}
			}
		})
	}
}
