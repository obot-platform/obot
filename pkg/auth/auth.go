package auth

import (
	"context"
)

// GroupInfo represents information about a user group from an authentication provider
type GroupInfo struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	IconURL *string `json:"iconURL"`
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
