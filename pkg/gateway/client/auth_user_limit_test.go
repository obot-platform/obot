package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type entitlementsFunc func(context.Context) (types.Entitlements, error)

func (f entitlementsFunc) Entitlements(ctx context.Context) (types.Entitlements, error) {
	return f(ctx)
}

func TestUserDecoratorResolveUserLimit(t *testing.T) {
	tests := []struct {
		name    string
		users   *int64
		want    UserLimit
		wantErr bool
	}{
		{
			name:  "bounded",
			users: new(int64(100)),
			want:  UserLimit{Maximum: 100},
		},
		{
			name: "no limit is unlimited",
			want: UserLimit{Unlimited: true},
		},
		{
			name:  "zero allows no new users",
			users: new(int64(0)),
			want:  UserLimit{},
		},
		{
			name:    "negative maximum",
			users:   new(int64(-1)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decorator := UserDecorator{
				entitlements: entitlementsFunc(func(context.Context) (types.Entitlements, error) {
					return types.Entitlements{Users: tt.users}, nil
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
			if got != tt.want {
				t.Fatalf("resolveUserLimit() = %+v, want %+v", got, tt.want)
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
		entitlementsFunc(func(context.Context) (types.Entitlements, error) {
			return types.Entitlements{Users: new(int64(1))}, currentErr
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
