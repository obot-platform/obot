package profile

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// UserProfile represents extracted user information from Entra ID
type UserProfile struct {
	OID               string // Object ID - stable user identifier in Azure AD
	Email             string
	PreferredUsername string
	DisplayName       string
	TenantID          string
}

// EntraIDTokenClaims represents the claims in an Entra ID token
type EntraIDTokenClaims struct {
	OID               string `json:"oid"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	UPN               string `json:"upn"` // User Principal Name - often in idToken when preferred_username is not
	Name              string `json:"name"`
	TenantID          string `json:"tid"`
	jwt.RegisteredClaims
}

// ParseIDToken extracts user information from an Entra ID ID token (JWT)
// The token is parsed without verification since oauth2-proxy already verified it
func ParseIDToken(idToken string) (*UserProfile, error) {
	if idToken == "" {
		return nil, fmt.Errorf("empty ID token")
	}

	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(idToken, &EntraIDTokenClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	claims, ok := token.Claims.(*EntraIDTokenClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract claims from ID token")
	}

	if claims.OID == "" {
		return nil, fmt.Errorf("no oid claim found in ID token")
	}

	// Use preferred_username if available, otherwise fall back to UPN
	// Azure AD often puts UPN in idToken and preferred_username in accessToken
	preferredUsername := claims.PreferredUsername
	if preferredUsername == "" {
		preferredUsername = claims.UPN
	}

	return &UserProfile{
		OID:               claims.OID,
		Email:             claims.Email,
		PreferredUsername: preferredUsername,
		DisplayName:       claims.Name,
		TenantID:          claims.TenantID,
	}, nil
}
