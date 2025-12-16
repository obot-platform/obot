package profile

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/tools/auth-providers-common/pkg/ratelimit"
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
// The token is parsed without verification since oauth2-proxy already verified it.
//
// SECURITY NOTE: We use ParseUnverified because the token signature has already
// been validated by oauth2-proxy during the OIDC authentication flow. This is
// safe because:
//  1. oauth2-proxy validates the signature using the IdP's public keys (JWKS)
//  2. oauth2-proxy validates standard claims (exp, iss, aud)
//  3. We only extract claims for internal use after authentication succeeds
//
// DO NOT use ParseUnverified for tokens that haven't been pre-validated.
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

// FetchUserIconURL fetches the user's profile picture from Microsoft Graph API
// and returns it as a base64-encoded data URL that can be used directly in browsers.
// accessToken should be the raw access token (not "Bearer <token>")
// Returns a data URL (data:image/jpeg;base64,...) if the user has a profile picture, empty string if not
func FetchUserIconURL(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", fmt.Errorf("no access token provided")
	}

	// Microsoft Graph API endpoint for user photo binary data
	// Using $value endpoint to get the actual image bytes
	photoURL := "https://graph.microsoft.com/v1.0/me/photo/$value"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, photoURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := ratelimit.DoWithRetry(ctx, graphClient, req, ratelimit.DefaultConfig())
	if err != nil {
		return "", fmt.Errorf("failed to fetch photo from Graph API: %w", err)
	}
	defer resp.Body.Close()

	// If photo doesn't exist (404), return empty string (not an error)
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("graph API photo returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read the image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read photo data: %w", err)
	}

	// Get content type from response, default to jpeg if not specified
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Encode as base64 data URL that browsers can display directly
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(imageData))

	return dataURL, nil
}
