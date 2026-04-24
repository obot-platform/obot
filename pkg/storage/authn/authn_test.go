package authn

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
)

func TestAuthenticateRequestAdminToken(t *testing.T) {
	authenticator := NewAuthenticator("admin-token")
	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer admin-token")

	resp, ok, err := authenticator.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected admin token to authenticate")
	}
	if resp.User.GetName() != "admin" {
		t.Fatalf("expected admin user, got %q", resp.User.GetName())
	}
}

func TestAuthenticateRequestServiceAccount(t *testing.T) {
	authenticator := NewAuthenticator("admin-token")
	authenticator.SetServiceAccountValidator(func(_ context.Context, token string) (string, error) {
		if token == "service-account-token" {
			return serviceaccounts.NetworkPolicyProvider, nil
		}
		return "", ErrInvalidServiceAccountToken
	})

	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer service-account-token")

	resp, ok, err := authenticator.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected service account token to authenticate")
	}
	if resp.User.GetName() != "system:serviceaccount:"+serviceaccounts.NetworkPolicyProvider {
		t.Fatalf("unexpected service account username %q", resp.User.GetName())
	}
}

func TestAuthenticateRequestRejectsInvalidServiceAccountToken(t *testing.T) {
	authenticator := NewAuthenticator("admin-token")
	authenticator.SetServiceAccountValidator(func(_ context.Context, token string) (string, error) {
		return "", fmt.Errorf("%w %s", ErrInvalidServiceAccountToken, token)
	})

	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer bad-token")

	if _, ok, err := authenticator.AuthenticateRequest(req); err != nil || ok {
		t.Fatalf("expected invalid token to be rejected, ok=%v err=%v", ok, err)
	}
}

func TestAuthenticateRequestPropagatesServiceAccountValidatorError(t *testing.T) {
	authenticator := NewAuthenticator("admin-token")
	authenticator.SetServiceAccountValidator(func(_ context.Context, token string) (string, error) {
		return "", fmt.Errorf("validator unavailable for %s", token)
	})

	req, err := http.NewRequest(http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer service-account-token")

	if _, ok, err := authenticator.AuthenticateRequest(req); err == nil || ok {
		t.Fatalf("expected validator error to propagate, ok=%v err=%v", ok, err)
	}
}
