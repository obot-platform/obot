package proxy

import (
	"net/http"
	"strings"
)

// IsSessionError determines if an error indicates an invalid or expired session.
// These errors should trigger a redirect to login rather than returning a 500 error.
func IsSessionError(statusCode int, body string) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}

	// List of error patterns that indicate invalid/expired session
	// These patterns are from:
	// - oauth2-proxy session validation failures
	// - Token refresh errors from state.go
	// - OAuth2 standard error codes
	sessionErrorPatterns := []string{
		"record not found",                        // Session not found in storage
		"session ticket cookie failed validation", // Cookie decryption/validation failure
		"refreshing token returned",               // Token refresh HTTP errors (401, 500, etc.)
		"REFRESH_TOKEN_ERROR",                     // OAuth2-proxy token refresh error
		"RESTART_AUTHENTICATION_ERROR",            // OAuth2-proxy auth restart error
		"invalid_token",                           // OAuth2 RFC 6749 standard error code
		"failed to refresh token",                 // State.go refresh error message
	}

	for _, pattern := range sessionErrorPatterns {
		if strings.Contains(body, pattern) {
			return true
		}
	}

	return false
}
