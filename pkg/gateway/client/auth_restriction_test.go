package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type restrictionCheckerFunc func(context.Context) (bool, error)

func (f restrictionCheckerFunc) Restricted(ctx context.Context) (bool, error) {
	return f(ctx)
}

func TestUserDecoratorCheckRestriction(t *testing.T) {
	restrictionErr := errors.New("resource limits are unreadable")

	tests := []struct {
		name       string
		checker    RestrictionChecker
		role       apitypes.Role
		wantStatus int
		wantErr    error
	}{
		{
			name: "no checker never restricts",
			role: apitypes.RoleBasic,
		},
		{
			name:    "an unrestricted installation admits everyone",
			checker: restrictionCheckerFunc(func(context.Context) (bool, error) { return false, nil }),
			role:    apitypes.RoleBasic,
		},
		{
			name:       "a restricted installation refuses a basic user",
			checker:    restrictionCheckerFunc(func(context.Context) (bool, error) { return true, nil }),
			role:       apitypes.RoleBasic,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "a restricted installation refuses a power user",
			checker:    restrictionCheckerFunc(func(context.Context) (bool, error) { return true, nil }),
			role:       apitypes.RolePowerUserPlus,
			wantStatus: http.StatusForbidden,
		},
		{
			name:    "a restricted installation admits an admin",
			checker: restrictionCheckerFunc(func(context.Context) (bool, error) { return true, nil }),
			role:    apitypes.RoleAdmin,
		},
		{
			name:    "a restricted installation admits an owner",
			checker: restrictionCheckerFunc(func(context.Context) (bool, error) { return true, nil }),
			role:    apitypes.RoleOwner,
		},
		{
			name:    "an admin is admitted without consulting the checker",
			checker: restrictionCheckerFunc(func(context.Context) (bool, error) { return false, restrictionErr }),
			role:    apitypes.RoleAdmin,
		},
		{
			name:    "a failed check refuses a basic user",
			checker: restrictionCheckerFunc(func(context.Context) (bool, error) { return false, restrictionErr }),
			role:    apitypes.RoleBasic,
			wantErr: restrictionErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decorator := UserDecorator{restrictionChecker: tt.checker}

			err := decorator.checkRestriction(t.Context(), tt.role)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("checkRestriction() error = %v, want %v", err, tt.wantErr)
				}
			case tt.wantStatus != 0:
				var httpErr *apitypes.ErrHTTP
				if !errors.As(err, &httpErr) {
					t.Fatalf("checkRestriction() error = %v, want *types.ErrHTTP", err)
				}
				if httpErr.Code != tt.wantStatus {
					t.Fatalf("checkRestriction() status = %d, want %d", httpErr.Code, tt.wantStatus)
				}
			default:
				if err != nil {
					t.Fatalf("checkRestriction() error = %v, want nil", err)
				}
			}
		})
	}
}

func TestUserDecoratorRefusesSignInWhileRestricted(t *testing.T) {
	c := newIdentityUserLimitTestClient(t)

	decorator := NewUserDecorator(
		authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{
				User: &user.DefaultInfo{
					Name: "user-1",
					UID:  "user-1",
					Extra: map[string][]string{
						"email":                   {"user-1@example.com"},
						"auth_provider_name":      {"test-auth-provider"},
						"auth_provider_namespace": {"default"},
					},
				},
			}, true, nil
		}),
		c,
		userLimitProviderFunc(func(context.Context) (UserLimit, error) {
			return UserLimit{Unlimited: true}, nil
		}),
		restrictionCheckerFunc(func(context.Context) (bool, error) { return true, nil }),
	)

	_, ok, err := decorator.AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if ok {
		t.Fatal("authentication succeeded while the installation is restricted")
	}

	var httpErr *apitypes.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("AuthenticateRequest() error = %v, want *types.ErrHTTP", err)
	}
	if httpErr.Code != http.StatusForbidden {
		t.Fatalf("AuthenticateRequest() status = %d, want %d", httpErr.Code, http.StatusForbidden)
	}
}
