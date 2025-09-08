package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
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

// GetSessionIDFromCookieValue extracts the session ID from a cookie value string.
// The cookie value should be an oauth2-proxy ticket cookie with three segments separated by pipes.
func GetSessionIDFromCookieValue(cookieValue string) string {
	// If the cookie is an oauth2-proxy ticket cookie, it should be three segments separated by pipes.
	// The first one contains the session ID.
	parts := strings.Split(cookieValue, "|")
	if len(parts) != 3 {
		return ""
	}

	firstPart, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}

	// This first part, after decoding, is three parts, separated by dots.
	// The middle one is the session ID encoded in base64.
	parts = strings.Split(string(firstPart), ".")
	if len(parts) != 3 {
		return ""
	}

	// Strangely, the session ID is usually not quite complete.
	// I think it gets truncated at some point. So we have to ignore errors when decoding.
	// We will still get part of the decoded session ID, and it's a long enough prefix to work.
	decodedID, _ := base64.StdEncoding.DecodeString(parts[1])
	// If it's not at least 10 characters, we can't really use it.
	// I've never seen this happen in testing, but it's best to be safe.
	if len(decodedID) < 10 {
		return ""
	}

	return string(decodedID)
}

// GetSessionInfoFromRequest extracts the session ID and cookie value from the request's obot access token cookie.
func GetSessionInfoFromRequest(req *http.Request) (sessionID, sessionCookie string) {
	cookie, err := req.Cookie(ObotAccessTokenCookie)
	if err != nil {
		return
	}

	sessionID = GetSessionIDFromCookieValue(cookie.Value)
	sessionCookie = cookie.Value
	return
}

// FirstExtraValue returns the first value for the given key in the extra map.
func FirstExtraValue(extra map[string][]string, key string) string {
	values := extra[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
