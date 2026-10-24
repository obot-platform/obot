package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

type registrySettingsStub struct {
	enabled bool
	err     error
}

type mutableRegistrySettings struct{ registrySettingsStub }

func TestRegistryRouteAuthorization(t *testing.T) {
	tests := []struct {
		name                          string
		enableAnonymousRegistryAccess bool
		user                          user.Info
		allowed                       bool
	}{
		{
			name:                          "auth mode rejects unauthenticated user",
			enableAnonymousRegistryAccess: false,
			user: &user.DefaultInfo{
				Name:   "anonymous",
				UID:    "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			allowed: false,
		},
		{
			name:                          "auth mode allows basic user",
			enableAnonymousRegistryAccess: false,
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:                          "auth mode allows MCP OAuth user",
			enableAnonymousRegistryAccess: false,
			user: &user.DefaultInfo{
				UID:    "user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: true,
		},
		{
			name:                          "anonymous access enabled allows unauthenticated user",
			enableAnonymousRegistryAccess: true,
			user: &user.DefaultInfo{
				Name:   "anonymous",
				UID:    "anonymous",
				Groups: []string{UnauthenticatedGroup},
			},
			allowed: true,
		},
		{
			name:                          "anonymous access enabled retains MCP OAuth access",
			enableAnonymousRegistryAccess: true,
			user: &user.DefaultInfo{
				UID:    "user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorizer := NewAuthorizer(nil, nil, nil, false, nil, nil, registrySettingsStub{enabled: tt.enableAnonymousRegistryAccess})
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				for _, path := range []string{
					"/v0.1/servers",
					"/v0.1/servers/com.example%2Fvmcp1example/versions",
					"/v0.1/servers/com.example%2Fvmcp1example/versions/latest",
				} {
					req := httptest.NewRequest(method, path, nil)
					if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
						t.Fatalf("Authorize(%s %s) = %v, want %v", method, path, allowed, tt.allowed)
					}
				}
			}
		})
	}
}

func (s registrySettingsStub) AnonymousRegistryAccessEnabled(context.Context) (bool, error) {
	return s.enabled, s.err
}

func TestRegistrySettingsChangeWithoutRestart(t *testing.T) {
	settings := &mutableRegistrySettings{}
	a := NewAuthorizer(nil, nil, nil, false, nil, nil, settings)
	anonymous := &user.DefaultInfo{Groups: []string{UnauthenticatedGroup}}
	authenticated := &user.DefaultInfo{Groups: []string{types.GroupMCP}}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		settings.err = nil
		for _, enabled := range []bool{false, true, false} {
			settings.enabled = enabled
			for _, path := range []string{"/v0.1/servers?search=example&limit=1", "/v0.1/servers/example/versions", "/v0.1/servers/example/versions/latest"} {
				req := httptest.NewRequest(method, path, nil)
				if got := a.Authorize(req, anonymous); got != enabled {
					t.Fatalf("anonymous access = %v, want %v", got, enabled)
				}
				if !a.Authorize(req, authenticated) {
					t.Fatal("authenticated access lost")
				}
			}
		}
		settings.enabled = true
		settings.err = errors.New("settings unavailable")
		req := httptest.NewRequest(method, "/v0.1/servers", nil)
		if a.Authorize(req, anonymous) {
			t.Fatal("settings lookup error allowed anonymous access")
		}
		if !a.Authorize(req, authenticated) {
			t.Fatal("settings error blocked authenticated access")
		}
	}
	settings.err = nil
	for _, path := range []string{"/v0.10/servers", "/api/registry-settings"} {
		if a.Authorize(httptest.NewRequest(http.MethodGet, path, nil), anonymous) {
			t.Fatalf("anonymous grant escaped registry: %s", path)
		}
	}
	if a.Authorize(httptest.NewRequest(http.MethodPost, "/v0.1/servers", nil), anonymous) {
		t.Fatal("anonymous write allowed")
	}
}

func (s *mutableRegistrySettings) AnonymousRegistryAccessEnabled(ctx context.Context) (bool, error) {
	return s.registrySettingsStub.AnonymousRegistryAccessEnabled(ctx)
}

func TestRegistrySettingsRoles(t *testing.T) {
	a := NewAuthorizer(nil, nil, nil, false, nil, nil, nil)
	for _, group := range []string{types.GroupOwner, types.GroupAdmin, types.GroupAuditor, types.GroupBasic, UnauthenticatedGroup} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			want := group == types.GroupOwner || group == types.GroupAdmin
			if got := a.Authorize(httptest.NewRequest(method, "/api/registry-settings", nil), &user.DefaultInfo{Groups: []string{group}}); got != want {
				t.Fatalf("%s %s allowed = %v, want %v", group, method, got, want)
			}
		}
	}
}
