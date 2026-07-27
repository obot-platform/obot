package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type userLimitProviderFunc func(context.Context) (UserLimit, error)

func (f userLimitProviderFunc) UserLimit(ctx context.Context) (UserLimit, error) {
	return f(ctx)
}

func TestUserDecoratorResolveUserLimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   UserLimit
		wantErr bool
	}{
		{
			name:  "bounded",
			limit: UserLimit{Maximum: 100},
		},
		{
			name:  "unlimited ignores maximum",
			limit: UserLimit{Unlimited: true},
		},
		{
			name:    "zero maximum",
			limit:   UserLimit{},
			wantErr: true,
		},
		{
			name:    "negative maximum",
			limit:   UserLimit{Maximum: -1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decorator := UserDecorator{
				userLimitProvider: userLimitProviderFunc(func(context.Context) (UserLimit, error) {
					return tt.limit, nil
				}),
			}

			got, err := decorator.resolveUserLimit(t.Context())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveUserLimit() = %+v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveUserLimit() error = %v", err)
			}
			if got != tt.limit {
				t.Fatalf("resolveUserLimit() = %+v, want %+v", got, tt.limit)
			}
		})
	}
}

func TestUserDecoratorDoesNotCreateUserWhenUserLimitProviderFails(t *testing.T) {
	c := newIdentityUserLimitTestClient(t)
	providerErr := errors.New("license lookup failed")
	currentErr := providerErr

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
			return UserLimit{Maximum: 1}, currentErr
		}),
	)

	_, ok, err := decorator.AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if !errors.Is(err, providerErr) {
		t.Fatalf("error = %v, want %v", err, providerErr)
	}
	if ok {
		t.Fatal("authentication succeeded despite user-limit provider failure")
	}
	if got := countIdentityUserLimitTestUsers(t, c, false); got != 0 {
		t.Fatalf("users = %d, want 0 after provider failure", got)
	}
	if got := countIdentityUserLimitTestIdentities(t, c); got != 0 {
		t.Fatalf("identities = %d, want 0 after provider failure", got)
	}

	currentErr = nil
	if _, ok, err := decorator.AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)); err != nil {
		t.Fatalf("authenticating after provider recovery: %v", err)
	} else if !ok {
		t.Fatal("authentication did not succeed after provider recovery")
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != 1 {
		t.Fatalf("users counted toward limit = %d, want 1", got)
	}
}
