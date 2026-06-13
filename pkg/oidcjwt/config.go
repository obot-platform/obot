package oidcjwt

import "strings"

type Config struct {
	IssuerURL            string
	Audience             string
	EligibilityClaimName string
	RolesClaimName       string
	AdminRoles           []string
}

const (
	defaultEligibilityClaimName = "eligible"
	defaultRolesClaimName       = "roles"
)

var defaultAdminRoles = []string{"admin"}

func NormalizeIssuer(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}

func (c Config) Enabled() bool {
	return c.IssuerURL != "" && c.Audience != ""
}

func LoadConfigFromEnv(getenv func(string) string) (Config, error) {
	issuer := getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER")
	cfg := Config{
		IssuerURL:            NormalizeIssuer(issuer),
		Audience:             getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_AUDIENCE"),
		EligibilityClaimName: getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ELIGIBILITY_CLAIM_NAME"),
		RolesClaimName:       getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ROLES_CLAIM_NAME"),
	}
	if cfg.EligibilityClaimName == "" {
		cfg.EligibilityClaimName = defaultEligibilityClaimName
	}
	if cfg.RolesClaimName == "" {
		cfg.RolesClaimName = defaultRolesClaimName
	}

	adminRolesStr := getenv("OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ADMIN_ROLES")
	if adminRolesStr == "" {
		cfg.AdminRoles = defaultAdminRoles
	} else {
		for _, r := range strings.Split(adminRolesStr, ",") {
			if trimmed := strings.TrimSpace(r); trimmed != "" {
				cfg.AdminRoles = append(cfg.AdminRoles, trimmed)
			}
		}
	}
	return cfg, nil
}
