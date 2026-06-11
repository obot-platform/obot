package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	GenericOAuthAuthProviderName        = "generic-oauth-auth-provider"
	GenericOAuthIssuerEnvVar            = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER"
	GenericOAuthEmailDomainsEnvVar      = "OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"
	GenericOAuthTrustEmailLinkingEnvVar = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING"
)

type oidcDiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	UserInfoEndpoint      string `json:"userinfo_endpoint"`
}

func validateGenericOAuthConfig(ctx context.Context, providerName string, envVars map[string]string) error {
	if providerName != GenericOAuthAuthProviderName {
		return nil
	}

	issuer := strings.TrimRight(strings.TrimSpace(envVars[GenericOAuthIssuerEnvVar]), "/")
	if issuer == "" {
		return fmt.Errorf("%s is required", GenericOAuthIssuerEnvVar)
	}
	if strings.TrimSpace(envVars[GenericOAuthEmailDomainsEnvVar]) == "" {
		return fmt.Errorf("%s is required", GenericOAuthEmailDomainsEnvVar)
	}

	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("%s must be a valid URL", GenericOAuthIssuerEnvVar)
	}
	if !genericOAuthIssuerAllowsHTTP(u.Hostname(), u.Scheme) {
		return fmt.Errorf("%s must use https", GenericOAuthIssuerEnvVar)
	}

	discoveryURL := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return err
	}

	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}

	var doc oidcDiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to parse OIDC discovery document: %w", err)
	}

	if strings.TrimRight(doc.Issuer, "/") != issuer {
		return fmt.Errorf("OIDC discovery issuer %q does not match configured issuer %q", doc.Issuer, issuer)
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.JWKSURI == "" {
		return fmt.Errorf("OIDC discovery document is missing required endpoints")
	}

	envVars[GenericOAuthIssuerEnvVar] = issuer
	return nil
}

func genericOAuthIssuerAllowsHTTP(hostname, scheme string) bool {
	if scheme == "https" {
		return true
	}
	if scheme != "http" {
		return false
	}
	return hostname == "localhost" || hostname == "127.0.0.1" || hostname == "host.docker.internal"
}

func requireGenericOAuthTrustReconfirmation(providerName, existingIssuer string, envVars map[string]string) error {
	if providerName != GenericOAuthAuthProviderName || existingIssuer == "" {
		return nil
	}

	newIssuer := envVars[GenericOAuthIssuerEnvVar]
	if newIssuer == existingIssuer {
		return nil
	}
	if envVars[GenericOAuthTrustEmailLinkingEnvVar] == "true" {
		return nil
	}

	return fmt.Errorf("issuer changed from %q to %q; account-linking trust must be re-confirmed", existingIssuer, newIssuer)
}
