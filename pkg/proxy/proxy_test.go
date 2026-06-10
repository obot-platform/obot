package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticateRequestPassesGenericOAuthMetadata(t *testing.T) {
	emailVerified := true
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-get-state" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}

		if err := json.NewEncoder(w).Encode(serializableState{
			AccessToken:       "access-token",
			PreferredUsername: "alice@example.com",
			User:              "iss:https://issuer.example.com/\x00sub:alice",
			Email:             "alice@example.com",
			Issuer:            "https://issuer.example.com/",
			EmailVerified:     &emailVerified,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer provider.Close()

	p, err := newProxy("default", "generic-oauth-auth-provider", provider.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://obot.example.com/", nil)
	resp, ok, err := p.authenticateRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected authentication to succeed")
	}

	extra := resp.User.GetExtra()
	if got := extra["auth_provider_issuer"]; len(got) != 1 || got[0] != "https://issuer.example.com/" {
		t.Fatalf("expected auth_provider_issuer extra, got %#v", got)
	}
	if got := extra["auth_provider_email_verified"]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("expected auth_provider_email_verified extra, got %#v", got)
	}
}
