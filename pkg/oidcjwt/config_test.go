package oidcjwt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfigFromEnv_AllFieldsPresent(t *testing.T) {
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":                 "https://studio.example.com/api/auth",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE":               "obot-default",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME": "eligible",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME":       "roles",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES":            "admin,owner",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.Equal(t, "https://studio.example.com/api/auth", cfg.IssuerURL)
	assert.Equal(t, "obot-default", cfg.Audience)
	assert.Equal(t, "eligible", cfg.EligibilityClaimName)
	assert.Equal(t, "roles", cfg.RolesClaimName)
	assert.Equal(t, []string{"admin", "owner"}, cfg.AdminRoles)
	assert.True(t, cfg.Enabled())
}

func TestLoadConfigFromEnv_DefaultsAndDisabled(t *testing.T) {
	env := map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER": "https://studio.example.com/api/auth",
	}
	cfg, err := LoadConfigFromEnv(envGetter(env))
	require.NoError(t, err)
	assert.False(t, cfg.Enabled())
	assert.Equal(t, "eligible", cfg.EligibilityClaimName)
	assert.Equal(t, "roles", cfg.RolesClaimName)
	assert.Equal(t, []string{"admin"}, cfg.AdminRoles)
}

func TestNormalizeIssuer(t *testing.T) {
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("https://issuer.example.com/"))
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("  https://issuer.example.com/  "))
	assert.Equal(t, "https://issuer.example.com", NormalizeIssuer("https://issuer.example.com//"))
}

func TestLoadConfigFromEnv_NormalizesIssuer(t *testing.T) {
	cfg, err := LoadConfigFromEnv(envGetter(map[string]string{
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":   "https://issuer.example.com/",
		"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE": "obot-default",
	}))
	require.NoError(t, err)
	assert.Equal(t, "https://issuer.example.com", cfg.IssuerURL)
}

func envGetter(env map[string]string) func(string) string {
	return func(k string) string { return env[k] }
}
