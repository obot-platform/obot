package profile

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// graphClient for Microsoft Graph API requests
var graphClient = &http.Client{
	Timeout: 30 * time.Second,
}

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

// FetchUserIconURL fetches the user's profile picture URL from Microsoft Graph API
// accessToken should be the raw access token (not "Bearer <token>")
// Returns the Graph API photo URL if the user has a profile picture, empty string if not
func FetchUserIconURL(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", fmt.Errorf("no access token provided")
	}

	// Microsoft Graph API endpoint for user photo metadata
	photoMetadataURL := "https://graph.microsoft.com/v1.0/me/photo"

	// Check if photo exists by calling the metadata endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoMetadataURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := graphClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch photo metadata from Graph API: %w", err)
	}
	defer resp.Body.Close()

	// If photo doesn't exist (404), return empty string (not an error)
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Graph API photo metadata returned status %d: %s", resp.StatusCode, string(body))
	}

	// Photo exists, return the $value URL that can be used to fetch the actual image
	return "https://graph.microsoft.com/v1.0/me/photo/$value", nil
}
