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

func TestValidateGenericOAuthConfigRejectsInvalidIssuerURL(t *testing.T) {
	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       "not a url",
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected invalid issuer URL to fail")
	}
}

func TestValidateGenericOAuthConfigRejectsInsecureRemoteIssuer(t *testing.T) {
	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       "http://issuer.example.com",
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected insecure remote issuer to fail")
	}
}

func TestGenericOAuthIssuerAllowsHTTPForLocalDockerHost(t *testing.T) {
	if !genericOAuthIssuerAllowsHTTP("host.docker.internal", "http") {
		t.Fatal("expected host.docker.internal over HTTP to be treated as local Docker")
	}
	if genericOAuthIssuerAllowsHTTP("issuer.example.com", "http") {
		t.Fatal("expected arbitrary HTTP issuers to remain blocked")
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

func TestValidateGenericOAuthConfigRejectsDiscoveryHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       server.URL,
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected discovery HTTP error to fail")
	}
}

func TestValidateGenericOAuthConfigRejectsMalformedDiscovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{`))
	}))
	t.Cleanup(server.Close)

	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       server.URL,
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected malformed discovery document to fail")
	}
}

func TestValidateGenericOAuthConfigRejectsMissingRequiredEndpoints(t *testing.T) {
	issuer := newOIDCDiscoveryServerWithDocument(t, func(issuer string) map[string]string {
		return map[string]string{
			"issuer":            issuer,
			"token_endpoint":    issuer + "/token",
			"jwks_uri":          issuer + "/jwks",
			"userinfo_endpoint": issuer + "/userinfo",
		}
	})

	err := validateGenericOAuthConfig(context.Background(), GenericOAuthAuthProviderName, map[string]string{
		GenericOAuthIssuerEnvVar:       issuer,
		GenericOAuthEmailDomainsEnvVar: "*",
	})
	if err == nil {
		t.Fatal("expected missing authorization endpoint to fail")
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

	return newOIDCDiscoveryServerWithDocument(t, func(issuer string) map[string]string {
		responseIssuer := issuer
		if overrideIssuer != "" {
			responseIssuer = overrideIssuer
		}

		return map[string]string{
			"issuer":                 responseIssuer,
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/jwks",
			"userinfo_endpoint":      issuer + "/userinfo",
		}
	})
}

func newOIDCDiscoveryServerWithDocument(t *testing.T, document func(issuer string) map[string]string) string {
	t.Helper()

	var issuer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}

		if err := json.NewEncoder(w).Encode(document(issuer)); err != nil {
			t.Fatal(err)
		}
	}))
	t.Cleanup(server.Close)
	issuer = server.URL
	return issuer
}
