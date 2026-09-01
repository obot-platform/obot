package auth

import (
	"context"
	"time"
)

const (
	ObotAccessTokenCookie = "obot_access_token"

	// AuthProviderVerifyCookie carries the ID of an in-flight staged-provider verification across
	// the OAuth round trip, so only the browser that started it can log in through the staged
	// provider.
	AuthProviderVerifyCookie = "obot_auth_provider_verify"

	// AuthProviderVerifyWindow is how long a staged-provider verification stays open.
	AuthProviderVerifyWindow = 15 * time.Minute
)

// SerializableRequest represents an HTTP request that can be serialized for authentication flows
type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// SerializableState represents the authentication state returned from auth providers
type SerializableState struct {
	ExpiresOn             *time.Time `json:"expiresOn"`
	AccessToken           string     `json:"accessToken"`
	PreferredUsername     string     `json:"preferredUsername"`
	User                  string     `json:"user"`
	Email                 string     `json:"email"`
	SetCookies            []string   `json:"setCookies"`
	RequirePasswordChange bool       `json:"requirePasswordChange,omitempty"`
}

// GroupInfo represents information about a user group from an authentication provider
type GroupInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconURL"`
}

type authProviderURLKey struct{}
type authProviderGroupIDPrefixKey struct{}

// ContextWithProviderURL adds the auth provider URL to the context
func ContextWithProviderURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, authProviderURLKey{}, url)
}

// ProviderURLFromContext retrieves the auth provider URL from the context
func ProviderURLFromContext(ctx context.Context) string {
	url, _ := ctx.Value(authProviderURLKey{}).(string)
	return url
}

// ContextWithProviderGroupIDPrefix adds the auth provider's declared group ID prefix to the context.
func ContextWithProviderGroupIDPrefix(ctx context.Context, prefix string) context.Context {
	return context.WithValue(ctx, authProviderGroupIDPrefixKey{}, prefix)
}

// ProviderGroupIDPrefixFromContext retrieves the auth provider's declared group ID prefix.
func ProviderGroupIDPrefixFromContext(ctx context.Context) string {
	prefix, _ := ctx.Value(authProviderGroupIDPrefixKey{}).(string)
	return prefix
}
