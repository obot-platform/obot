package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"time"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authProviderIDEnvVar                = "OBOT_AUTH_PROVIDER_ID"
	genericOAuthAuthProviderName        = "generic-oauth-auth-provider"
	genericOAuthProviderNameEnvVar      = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME"
	genericOAuthIssuerEnvVar            = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER"
	genericOAuthClientIDEnvVar          = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID"
	genericOAuthClientSecretEnvVar      = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET"
	genericOAuthScopeEnvVar             = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE"
	genericOAuthTrustEmailLinkingEnvVar = "OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING"
	authProviderCookieSecretEnv         = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"
	authProviderEmailDomainsEnv         = "OBOT_AUTH_PROVIDER_EMAIL_DOMAINS"
	authProviderPostgresDSNEnv          = "OBOT_AUTH_PROVIDER_POSTGRES_CONNECTION_DSN"
	authProviderRefreshPeriodEnv        = "OBOT_AUTH_PROVIDER_TOKEN_REFRESH_DURATION"
	authProviderLoggingEnv              = "OBOT_AUTH_PROVIDER_ENABLE_LOGGING"
)

var legacyGenericOAuthStartupRequiredEnvVars = []string{
	genericOAuthIssuerEnvVar,
	genericOAuthClientIDEnvVar,
	genericOAuthClientSecretEnvVar,
}

var legacyGenericOAuthStartupOptionalEnvVars = []string{
	genericOAuthProviderNameEnvVar,
	genericOAuthScopeEnvVar,
	genericOAuthTrustEmailLinkingEnvVar,
	authProviderCookieSecretEnv,
	authProviderEmailDomainsEnv,
	authProviderPostgresDSNEnv,
	authProviderRefreshPeriodEnv,
	authProviderLoggingEnv,
}

// EnsureAuthProviderEnvCredential configures one registry-backed auth provider
// from process env at startup. OBOT_AUTH_PROVIDER_ID selects the provider, and
// that provider manifest defines the required/optional config env names.
func (h *Handler) EnsureAuthProviderEnvCredential(ctx context.Context, c kclient.Client) error {
	providerID, configured := authProviderEnvID(os.LookupEnv)
	if !configured {
		return nil
	}

	authProvider, err := waitForAuthProvider(ctx, c, providerID)
	if err != nil {
		return err
	}

	secrets, err := authProviderEnvSecrets(authProvider, os.LookupEnv)
	if err != nil {
		return err
	}

	existing, err := h.gatewayClient.RevealCredential(ctx, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil && !isCredentialNotFound(err) {
		return fmt.Errorf("failed to reveal existing auth provider credential %q: %w", authProvider.Name, err)
	}
	if err == nil {
		if cookieSecret := strings.TrimSpace(existing.Secrets[authProviderCookieSecretEnv]); cookieSecret != "" {
			secrets[authProviderCookieSecretEnv] = cookieSecret
		}
		if maps.Equal(existing.Secrets, secrets) {
			return nil
		}
	}
	if strings.TrimSpace(secrets[authProviderCookieSecretEnv]) == "" {
		cookieSecret, err := generateAuthProviderCookieSecret()
		if err != nil {
			return err
		}
		secrets[authProviderCookieSecretEnv] = cookieSecret
	}

	if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: secrets,
	}); err != nil {
		return fmt.Errorf("failed to upsert auth provider credential %q: %w", authProvider.Name, err)
	}

	if authProvider.Annotations == nil {
		authProvider.Annotations = map[string]string{}
	}
	if authProvider.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		authProvider.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(authProvider.Annotations, v1.AuthProviderSyncAnnotation)
	}
	if err := c.Update(ctx, &authProvider); err != nil {
		return fmt.Errorf("failed to update auth provider sync annotation %q: %w", authProvider.Name, err)
	}

	log.Infof("Configured auth provider from environment: provider=%s", authProvider.Name)
	return nil
}

func authProviderEnvID(lookup func(string) (string, bool)) (string, bool) {
	if value, ok := lookup(authProviderIDEnvVar); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}

	for _, key := range append(append([]string{}, legacyGenericOAuthStartupRequiredEnvVars...), legacyGenericOAuthStartupOptionalEnvVars...) {
		if value, ok := lookup(key); ok && strings.TrimSpace(value) != "" {
			return genericOAuthAuthProviderName, true
		}
	}

	return "", false
}

func waitForAuthProvider(ctx context.Context, c kclient.Client, providerID string) (v1.AuthProvider, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var authProvider v1.AuthProvider
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: providerID}, &authProvider)
		if err == nil {
			return authProvider, nil
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return v1.AuthProvider{}, fmt.Errorf("auth provider env is set but provider %q is not registered: %w", providerID, err)
		}
	}
}

func authProviderEnvSecrets(authProvider v1.AuthProvider, lookup func(string) (string, bool)) (map[string]string, error) {
	secrets := map[string]string{}
	applyGenericOAuthDefaults(authProvider.Name, secrets)

	for _, parameter := range authProvider.Spec.RequiredConfigurationParameters {
		value := envValue(lookup, parameter.Name)
		if value == "" && parameter.Name == authProviderCookieSecretEnv {
			continue
		}
		if value == "" {
			value = strings.TrimSpace(secrets[parameter.Name])
		}
		if value == "" {
			return nil, fmt.Errorf("%s must be set when auth provider env bootstrap is enabled for %s", parameter.Name, authProvider.Name)
		}
		secrets[parameter.Name] = value
	}

	for _, parameter := range authProvider.Spec.OptionalConfigurationParameters {
		if value := envValue(lookup, parameter.Name); value != "" {
			secrets[parameter.Name] = value
		}
	}

	return secrets, nil
}

func envValue(lookup func(string) (string, bool), key string) string {
	value, _ := lookup(key)
	return strings.TrimSpace(value)
}

func applyGenericOAuthDefaults(providerID string, secrets map[string]string) {
	if providerID != genericOAuthAuthProviderName {
		return
	}
	if strings.TrimSpace(secrets[genericOAuthProviderNameEnvVar]) == "" {
		secrets[genericOAuthProviderNameEnvVar] = "Custom OAuth"
	}
	if strings.TrimSpace(secrets[authProviderEmailDomainsEnv]) == "" {
		secrets[authProviderEmailDomainsEnv] = "*"
	}
	if strings.TrimSpace(secrets[genericOAuthTrustEmailLinkingEnvVar]) == "" {
		secrets[genericOAuthTrustEmailLinkingEnvVar] = "false"
	}
}

func generateAuthProviderCookieSecret() (string, error) {
	const length = 32
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate auth provider cookie secret: %w", err)
	}
	for len(bytes.TrimSpace(b)) != length {
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("failed to generate auth provider cookie secret: %w", err)
		}
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func isCredentialNotFound(err error) bool {
	var notFound gateway.CredentialNotFoundError
	return errors.As(err, &notFound)
}
