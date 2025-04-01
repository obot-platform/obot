// Package ratelimiter implements HTTP middleware for rate limiting API requests
// with different limits for authenticated and unauthenticated users.
// It supports extracting credentials from HTTP headers or cookies,
// caching authenticated users, and allowing admins to bypass rate limits entirely.
package ratelimit

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"k8s.io/apiserver/pkg/authentication/user"
)

// Middleware defines the interface for HTTP rate limiting middleware.
// Implementations of this interface can be used to wrap HTTP handlers
// with rate limiting functionality.
type Middleware interface {
	// Limit wraps an HTTP handler with rate limiting logic.
	// It returns a new handler that enforces rate limits based on user authentication.
	Limit(next http.Handler) http.Handler

	// RegisterUser adds or updates a user in the rate limiter's cache.
	// This makes subsequent requests with the same credentials use the authenticated rate.
	RegisterUser(u user.Info) error
}

// CredSourceType defines where credentials are stored in the HTTP request.
// Currently supports headers and cookies.
type CredSourceType string

// Supported credential source types
const (
	// CredSourceTypeHeader indicates credentials are in an HTTP header
	CredSourceTypeHeader CredSourceType = "header"

	// CredSourceTypeCookie indicates credentials are in an HTTP cookie
	CredSourceTypeCookie CredSourceType = "cookie"

	// Key used for storing credential info in user.Info.Extra map
	credInfoKey = "rate_limit_cred_info"
)

// CredSource defines a specific location to extract credentials from HTTP requests.
// It combines a type (header/cookie) with a name (e.g., "Authorization" or "session").
type CredSource struct {
	// Type specifies if the credential is in a header or cookie
	Type CredSourceType

	// Name is the specific header or cookie name containing the credential
	Name string
}

func (c *CredSource) extractCred(r *http.Request) string {
	var cred string

	switch c.Type {
	case CredSourceTypeHeader:
		cred = r.Header.Get(c.Name)
	case CredSourceTypeCookie:
		if cookie, err := r.Cookie(c.Name); err == nil {
			cred = cookie.Value
		}
	}

	return cred
}

// User represents an authenticated user's rate limiting metadata.
// It contains only the information needed for rate limiting decisions.
type User struct {
	// ID is the unique identifier for the user, used as the rate limit key
	ID string

	// IsAdmin determines if the user bypasses rate limits completely
	IsAdmin bool
}

// EnableGroupRateLimit adds credential information to a user.DefaultInfo for later retrieval.
// This should be called during the authentication process to store the credential
// source information that was used to authenticate the user.
func EnableAuthGroupRateLimit(sourceType CredSourceType, sourceName, cred string, u *user.DefaultInfo) {
	if u.Extra == nil {
		u.Extra = make(map[string][]string)
	}

	// Store the credential source type, name, and hashed token
	// Format: "type:name:hash" (e.g. "header:Authorization:abc123...")
	u.Extra[credInfoKey] = []string{
		fmt.Sprintf("%s:%s:%s", sourceType, sourceName, hashCred(cred)),
	}
}

// hashCred creates a cryptographic hash of a cred value.
// This provides security by avoiding storage of the actual credentials
// while still allowing lookups by credential.
func hashCred(cred string) string {
	h := sha256.New()
	h.Write([]byte(cred))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// getUserCredInfo gets the credential source and token hash from user.Extra.
// This information was previously stored during authentication via StoreCredInfo.
func getUserCredInfo(u user.Info) (credHash string, source CredSource, ok bool) {
	credInfos, exists := u.GetExtra()[credInfoKey]
	if !exists || len(credInfos) == 0 {
		return "", CredSource{}, false
	}

	parts := strings.Split(credInfos[0], ":")
	if len(parts) != 3 {
		return "", CredSource{}, false
	}

	return parts[2], CredSource{
		Type: CredSourceType(parts[0]),
		Name: parts[1],
	}, true
}
