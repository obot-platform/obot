package server

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/pkg/auth"
)

var (
	// headerCredentials are the request headers that carry a credential, such as a bearer token or an API key, that a
	// client sets itself. A browser never attaches one on its own.
	headerCredentials = []string{"Authorization", "X-API-Key", "X-Obot-Api-Key"}

	// defaultCrossOrigin is used by a Server that was not built by NewServer, such as in tests.
	defaultCrossOrigin = http.NewCrossOriginProtection()
)

// newCrossOriginProtection returns the protection that rejects cross-origin requests that a browser authenticated with
// a cookie. Obot's own URLs are trusted: the server URL, so that a request is accepted when a reverse proxy in front of
// Obot changes its Host header, and the UI URL (OBOT_SERVER_UI_HOSTNAME), whose pages call the API from their origin.
func newCrossOriginProtection(trustedURLs ...string) *http.CrossOriginProtection {
	protection := http.NewCrossOriginProtection()
	for _, trusted := range trustedURLs {
		u, err := url.Parse(trusted)
		if err != nil || u.Scheme == "" || u.Host == "" {
			if trusted != "" {
				slog.Warn("Not trusting an invalid URL for cross-origin requests", "url", trusted)
			}
			continue
		}
		if err := protection.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
			slog.Warn("Failed to trust a URL for cross-origin requests", "url", trusted, "error", err)
		}
	}
	return protection
}

// rejectCrossOrigin reports whether req is a state-changing request that another origin made a browser send with Obot's
// session or bootstrap cookie. SameSite cookies do not prevent this, because another origin on the same site, such as
// another tenant's subdomain, gets the browser to attach them.
//
// Only cookie authentication is checked. A request with a header credential is authenticated by it, and clients, such
// as MCP clients and CLIs, send those from any origin. A cross-origin page cannot add such a header to a request that
// carries cookies: the header requires a CORS preflight, and Obot's CORS policy never allows credentials (see
// pkg/server). A request without any of these cookies has no user's authority to abuse, and anonymous endpoints, such
// as OAuth token exchange, are called cross-origin by browser-based clients.
func (s *Server) rejectCrossOrigin(req *http.Request) bool {
	if !cookieAuthenticated(req) {
		return false
	}

	protection := s.crossOrigin
	if protection == nil {
		protection = defaultCrossOrigin
	}
	if err := protection.Check(req); err != nil {
		slog.Warn("Rejected a cross-origin request authenticated with a cookie",
			"method", req.Method,
			"path", req.URL.Path,
			"origin", req.Header.Get("Origin"),
			"secFetchSite", req.Header.Get("Sec-Fetch-Site"),
		)
		return true
	}
	return false
}

// cookieAuthenticated reports whether req can be authenticated only by a browser cookie: it carries Obot's session or
// bootstrap cookie and no header credential.
func cookieAuthenticated(req *http.Request) bool {
	for _, header := range headerCredentials {
		if req.Header.Get(header) != "" {
			return false
		}
	}
	for _, name := range []string{auth.ObotAccessTokenCookie, auth.ObotBootstrapCookie} {
		if _, err := req.Cookie(name); err == nil {
			return true
		}
	}
	return false
}
