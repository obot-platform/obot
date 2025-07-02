package authz

import (
	"net/http"
	"net/url"
	"testing"

	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCheckUI_V2AdminAccess(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		user     user.Info
		expected bool
	}{
		{
			name: "admin user can access /v2/admin/users",
			path: "/v2/admin/users",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{AdminGroup, AuthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "bootstrap user can access /v2/admin/auth-providers",
			path: "/v2/admin/auth-providers",
			user: &user.DefaultInfo{
				Name:   "bootstrap",
				Groups: []string{AdminGroup, AuthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "regular user cannot access /v2/admin/users",
			path: "/v2/admin/users",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{AuthenticatedGroup},
			},
			expected: false,
		},
		{
			name: "unauthenticated user can access /v2/admin",
			path: "/v2/admin",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "authenticated user can access /v2/admin",
			path: "/v2/admin",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{AuthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "unauthenticated user can access /v2/admin/",
			path: "/v2/admin/",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "authenticated user can access /v2/admin/",
			path: "/v2/admin/",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{AuthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "unauthenticated user cannot access /v2/admin/auth-providers",
			path: "/v2/admin/auth-providers",
			user: &user.DefaultInfo{
				Name:   "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			expected: false,
		},
		{
			name: "admin user can access regular UI paths",
			path: "/",
			user: &user.DefaultInfo{
				Name:   "admin",
				Groups: []string{AdminGroup, AuthenticatedGroup},
			},
			expected: true,
		},
		{
			name: "regular user can access regular UI paths",
			path: "/",
			user: &user.DefaultInfo{
				Name:   "user",
				Groups: []string{AuthenticatedGroup},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := &Authorizer{
				uiResources: newPathMatcher(uiResources...),
			}

			req := &http.Request{
				Method: "GET",
				URL:    &url.URL{Path: tt.path},
			}

			result := authorizer.checkUI(req, tt.user)
			if result != tt.expected {
				t.Errorf("checkUI() = %v, want %v", result, tt.expected)
			}
		})
	}
}
