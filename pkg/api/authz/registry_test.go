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
			authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, tt.registryNoAuth)
			req := httptest.NewRequest(http.MethodGet, "/v0.1/servers", nil)

			if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}
