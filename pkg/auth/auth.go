package auth

import (
	"context"
	"fmt"
	"time"
)

const (
	ObotAccessTokenCookie = "obot_access_token"
)

// SerializableRequest represents an HTTP request that can be serialized for authentication flows
type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// SerializableState represents the authentication state returned from auth providers
type SerializableState struct {
	ExpiresOn         *time.Time `json:"expiresOn"`
	AccessToken       string     `json:"accessToken"`
	PreferredUsername string     `json:"preferredUsername"`
	User              string     `json:"user"`
	Email             string     `json:"email"`
	SetCookies        []string   `json:"setCookies"`
}

// ProviderUsername returns the username for the given provider.
func (ss SerializableState) ProviderUsername(providerName string) string {
	// Important: do not change the order of these checks.
	// We rely on the preferred username from GitHub being the user ID in the sessions table.
	// See pkg/gateway/server/logout_all.go for more details, as well as the GitHub auth provider code.
	if providerName == "github-auth-provider" {
		return ss.PreferredUsername
	}

	userName := ss.User
	if userName == "" {
		userName = ss.Email
	}

	return userName
}

const (
	// MaxGroupDescriptionLength is the maximum allowed length for group descriptions
	// This prevents storage abuse and potential display issues
	MaxGroupDescriptionLength = 1000
)

// GroupInfo represents information about a user group from an authentication provider
type GroupInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	IconURL     *string `json:"iconURL,omitempty"`
}

// Validate validates group info fields for security and data integrity
func (g *GroupInfo) Validate() error {
	if g.Name == "" {
		return fmt.Errorf("group name is required")
	}

	if g.Description != nil && len(*g.Description) > MaxGroupDescriptionLength {
		return fmt.Errorf("description exceeds maximum length of %d characters (got %d)",
			MaxGroupDescriptionLength, len(*g.Description))
	}

	return nil
}

type authProviderURLKey struct{}

// ContextWithProviderURL adds the auth provider URL to the context
func ContextWithProviderURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, authProviderURLKey{}, url)
}

// ProviderURLFromContext retrieves the auth provider URL from the context
func ProviderURLFromContext(ctx context.Context) string {
	url, _ := ctx.Value(authProviderURLKey{}).(string)
	return url
}

// FirstExtraValue returns the first value for the given key in the extra map.
func FirstExtraValue(extra map[string][]string, key string) string {
	values := extra[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
