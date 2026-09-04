package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestAuthProviderRouteAuthorization(t *testing.T) {
	authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, nil, false)

	mutations := []string{
		"/api/auth-providers/google-auth-provider/configure",
		"/api/auth-providers/google-auth-provider/deconfigure",
	}

	ownerOnly := []string{
		"/api/auth-providers/google-auth-provider/stage",
		"/api/auth-providers/google-auth-provider/verify",
		"/api/auth-providers/google-auth-provider/activate",
	}

	users := []struct {
		name         string
		info         user.Info
		canMutate    bool
		canOwnerOnly bool
		canReveal    bool
	}{
		{
			name:         "owner",
			info:         &user.DefaultInfo{Name: "owner", Groups: types.RoleOwner.Groups()},
			canMutate:    true,
			canOwnerOnly: true,
			canReveal:    true,
		},
		{
			name:      "administrator",
			info:      &user.DefaultInfo{Name: "admin", Groups: types.RoleAdmin.Groups()},
			canMutate: true,
			canReveal: true,
		},
		{
			name:      "auditor",
			info:      &user.DefaultInfo{Name: "auditor", Groups: types.RoleAuditor.Groups()},
			canReveal: true,
		},
		{
			name: "basic user",
			info: &user.DefaultInfo{Name: "basic", Groups: types.RoleBasic.Groups()},
		},
	}

	for _, u := range users {
		t.Run(u.name, func(t *testing.T) {
			for _, path := range mutations {
				req := httptest.NewRequest(http.MethodPost, path, nil)
				if got := authorizer.Authorize(req, u.info); got != u.canMutate {
					t.Errorf("POST %s = %v, want %v", path, got, u.canMutate)
				}
			}

			for _, path := range ownerOnly {
				req := httptest.NewRequest(http.MethodPost, path, nil)
				if got := authorizer.Authorize(req, u.info); got != u.canOwnerOnly {
					t.Errorf("POST %s = %v, want %v", path, got, u.canOwnerOnly)
				}
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/auth-providers/google-auth-provider/stage", nil)
			if got := authorizer.Authorize(req, u.info); got != u.canOwnerOnly {
				t.Errorf("DELETE stage = %v, want %v", got, u.canOwnerOnly)
			}

			req = httptest.NewRequest(http.MethodPost, "/api/auth-providers/google-auth-provider/reveal", nil)
			if got := authorizer.Authorize(req, u.info); got != u.canReveal {
				t.Errorf("POST reveal = %v, want %v", got, u.canReveal)
			}
		})
	}
}
