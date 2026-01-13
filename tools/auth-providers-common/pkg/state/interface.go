package state

import (
	"net/http"

	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	sessionsapi "github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/sessions"
)

// SessionManager defines the interface for session management operations
// needed by the state package. This abstraction allows decoupling from
// the concrete OAuthProxy type which resides in package main and cannot
// be imported directly.
//
// Implementations of this interface should wrap the OAuthProxy instance
// and delegate to its methods.
type SessionManager interface {
	// LoadCookiedSession loads a session from the request cookies
	LoadCookiedSession(req *http.Request) (*sessionsapi.SessionState, error)

	// ServeHTTP handles HTTP requests (used for token refresh)
	ServeHTTP(w http.ResponseWriter, req *http.Request)

	// GetCookieOptions returns the cookie configuration
	GetCookieOptions() *options.Cookie
}
