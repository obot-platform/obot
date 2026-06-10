package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateGenericOAuthConfigRejectsMissingIssuer(t *testing.T) {
	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected missing issuer to fail")
	}
}

func TestValidateGenericOAuthConfigRejectsMissingEmailDomains(t *testing.T) {
	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar: "https://issuer.example.com",
	})
	if err == nil {
		t.Fatal("expected missing email domains to fail")
	}
}

func TestValidateGenericOAuthConfigRejectsIssuerMismatch(t *testing.T) {
	issuer := newOIDCDiscoveryServer(t, "https://other.example.com")

	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       issuer,
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected issuer mismatch to fail")
	}
}

func TestValidateGenericOAuthConfigAcceptsValidDiscovery(t *testing.T) {
	issuer := newOIDCDiscoveryServer(t, "")
	envVars := map[string]string{
		GenericOAuthIssuerEnvVar:       issuer + "/",
		GenericOAuthEmailDomainsEnvVar: "*",
	}

	if err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, envVars); err != nil {
		t.Fatal(err)
	}
	if envVars[GenericOAuthIssuerEnvVar] != issuer {
		t.Fatalf("expected canonical issuer %q, got %q", issuer, envVars[GenericOAuthIssuerEnvVar])
	}
}

func TestRequireGenericOAuthTrustReconfirmationRejectsIssuerChangeWithoutTrust(t *testing.T) {
	err := requireGenericOAuthTrustReconfirmation(GenericOAuthAuthProviderName, "https://old.example.com", map[string]string{
		GenericOAuthIssuerEnvVar: "https://new.example.com",
	})
	if err == nil {
		t.Fatal("expected issuer change without trust confirmation to fail")
	}
}

func TestRequireGenericOAuthTrustReconfirmationAllowsIssuerChangeWithTrust(t *testing.T) {
	err := requireGenericOAuthTrustReconfirmation(GenericOAuthAuthProviderName, "https://old.example.com", map[string]string{
		GenericOAuthIssuerEnvVar:            "https://new.example.com",
		GenericOAuthTrustEmailLinkingEnvVar: "true",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newOIDCDiscoveryServer(t *testing.T, overrideIssuer string) string {
	t.Helper()

	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}

		responseIssuer := issuer
		if overrideIssuer != "" {
			responseIssuer = overrideIssuer
		}

		if err := json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 responseIssuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
			"userinfo_endpoint":      issuer + "/userinfo",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)
	issuer = server.URL
	return issuer
}
