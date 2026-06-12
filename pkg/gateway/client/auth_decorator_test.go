package client

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestUserDecoratorPassesGenericOAuthIssuerAndEmailVerified(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://studio.example.com/api/auth", "true")
	existing, err := c.EnsureIdentity(t.Context(), &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	decorator := NewUserDecorator(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name: "studio-user",
				UID:  "studio-subject",
				Extra: map[string][]string{
					"email":                        {"alice@example.com"},
					"auth_provider_name":           {genericOAuthAuthProviderName},
					"auth_provider_namespace":      {system.DefaultNamespace},
					"auth_provider_issuer":         {"https://studio.example.com/api/auth"},
					"auth_provider_email_verified": {"true"},
				},
			},
		},
	}, c)

	resp, ok, err := decorator.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected decorated authentication response")
	}
	if resp.User.GetUID() != fmt.Sprintf("%d", existing.ID) {
		t.Fatalf("expected generic OAuth identity to link to existing user %d, got UID %q", existing.ID, resp.User.GetUID())
	}
	if got := resp.User.GetExtra()["auth_provider_groups"]; got == nil {
		t.Fatal("expected auth provider groups to be present in response extra")
	}
}

func TestUserDecoratorRejectsInvalidEmailVerifiedValue(t *testing.T) {
	c := newTestClient(t)
	decorator := NewUserDecorator(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name: "studio-user",
				UID:  "studio-subject",
				Extra: map[string][]string{
					"email":                        {"alice@example.com"},
					"auth_provider_name":           {genericOAuthAuthProviderName},
					"auth_provider_namespace":      {system.DefaultNamespace},
					"auth_provider_email_verified": {"not-bool"},
				},
			},
		},
	}, c)

	resp, ok, err := decorator.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	if err == nil {
		t.Fatal("expected invalid auth_provider_email_verified to fail")
	}
	if ok {
		t.Fatal("expected authentication not to succeed")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %#v", resp)
	}
	if !strings.Contains(err.Error(), "invalid auth_provider_email_verified value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUserDecoratorNoopsWithoutAuthProviderMetadata(t *testing.T) {
	c := newTestClient(t)
	decorator := NewUserDecorator(staticAuthenticator{
		response: &authenticator.Response{
			User: &user.DefaultInfo{
				Name:  "api-key-user",
				UID:   "api-key-user",
				Extra: map[string][]string{},
			},
		},
	}, c)

	resp, ok, err := decorator.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected decorator to skip users without auth provider metadata")
	}
	if resp != nil {
		t.Fatalf("expected nil response, got %#v", resp)
	}
}

type staticAuthenticator struct {
	response *authenticator.Response
	err      error
}

func (s staticAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	if s.err != nil {
		return nil, false, s.err
	}
	if s.response == nil {
		return nil, false, nil
	}
	return s.response, true, nil
}
