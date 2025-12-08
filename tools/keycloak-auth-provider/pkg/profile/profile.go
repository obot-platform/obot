package profile

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// UserProfile represents extracted user information from Keycloak
type UserProfile struct {
	Subject           string   // Subject - stable user identifier in Keycloak
	Email             string
	PreferredUsername string
	Name              string
	Groups            []string // Groups from token claims (if configured in Keycloak)
	RealmRoles        []string // Realm-level roles
	ClientRoles       []string // Client-specific roles
}

// KeycloakTokenClaims represents the claims in a Keycloak ID token
type KeycloakTokenClaims struct {
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	// Groups claim - requires "groups" client scope with Group Membership mapper in Keycloak
	Groups []string `json:"groups"`
	// Realm access contains realm-level roles
	RealmAccess *RealmAccess `json:"realm_access"`
	// Resource access contains client-specific roles
	ResourceAccess map[string]*ResourceAccess `json:"resource_access"`
	jwt.RegisteredClaims
}

// RealmAccess contains realm-level role assignments
type RealmAccess struct {
	Roles []string `json:"roles"`
}

// ResourceAccess contains client-specific role assignments
type ResourceAccess struct {
	Roles []string `json:"roles"`
}

// ParseIDToken extracts user information from a Keycloak ID token (JWT)
// The token is parsed without verification since oauth2-proxy already verified it
func ParseIDToken(idToken string) (*UserProfile, error) {
	if idToken == "" {
		return nil, fmt.Errorf("empty ID token")
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(idToken, &KeycloakTokenClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	claims, ok := token.Claims.(*KeycloakTokenClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract claims from ID token")
	}

	// Subject (sub) claim is required in OIDC tokens
	if claims.Subject == "" {
		return nil, fmt.Errorf("no sub claim found in ID token")
	}

	profile := &UserProfile{
		Subject:           claims.Subject,
		Email:             claims.Email,
		PreferredUsername: claims.PreferredUsername,
		Name:              claims.Name,
		Groups:            claims.Groups,
	}

	// Extract realm roles if present
	if claims.RealmAccess != nil {
		profile.RealmRoles = claims.RealmAccess.Roles
	}

	// Extract client roles from all clients
	for _, access := range claims.ResourceAccess {
		if access != nil {
			profile.ClientRoles = append(profile.ClientRoles, access.Roles...)
		}
	}

	return profile, nil
}
