package server

import (
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/obot-platform/obot/pkg/auth"
	"golang.org/x/time/rate"
)

const (
	rejectionLogInterval = 10 * time.Second
)

var (
	obotCookies = []string{auth.ObotAccessTokenCookie, auth.ObotBootstrapCookie}

	// unconfiguredCrossOriginGuard is used by a Server that was not built by NewServer, such as in tests. It trusts no
	// origin but the request's own Host, and is shared, so that its rejections are logged at the same limited rate.
	unconfiguredCrossOriginGuard = newCrossOriginGuard()
)

// crossOriginGuard rejects cross-origin requests and logs them. It is shared by the servers that WithMiddleware returns.
type crossOriginGuard struct {
	protection http.CrossOriginProtection
	log        rejectionLog
}

// rejectionLog limits how often rejected cross-origin requests are logged as warnings.
type rejectionLog struct {
	warnings   rate.Sometimes
	suppressed atomic.Int64
}

// newCrossOriginGuard returns a guard that trusts Obot's own URLs: the server URL, so that a request is accepted when a
// reverse proxy in front of Obot changes its Host header, and the UI URL (OBOT_SERVER_UI_HOSTNAME), whose pages call
// the API from their origin.
func newCrossOriginGuard(trustedURLs ...string) *crossOriginGuard {
	g := &crossOriginGuard{log: newRejectionLog()}
	auth.TrustOrigins(&g.protection, trustedURLs...)
	return g
}

// reject reports whether req is a cross-origin request, and logs it if so. A nil guard is unconfiguredCrossOriginGuard.
func (g *crossOriginGuard) reject(req *http.Request) bool {
	if g == nil {
		g = unconfiguredCrossOriginGuard
	}
	if err := g.protection.Check(req); err != nil {
		g.log.log(req)
		return true
	}
	return false
}

func newRejectionLog() rejectionLog {
	return rejectionLog{warnings: rate.Sometimes{Interval: rejectionLogInterval}}
}

func (l *rejectionLog) log(req *http.Request) {
	attrs := []any{
		"method", req.Method,
		"path", req.URL.Path,
		"origin", req.Header.Get("Origin"),
		"secFetchSite", req.Header.Get("Sec-Fetch-Site"),
	}

	warned := false
	l.warnings.Do(func() {
		warned = true
		slog.Warn("Rejected a cross-origin request authenticated by the browser",
			append(attrs, "suppressedSinceLastWarning", l.suppressed.Swap(0))...)
	})
	if !warned {
		l.suppressed.Add(1)
		slog.Debug("Rejected a cross-origin request authenticated by the browser", attrs...)
	}
}

// rejectCrossOrigin reports whether req is a state-changing request that another origin made a browser send with Obot's
// session or bootstrap cookie.
//
// Only cookie authentication is checked. A request with a header credential, such as a bearer token, is authenticated
// by it, and clients, such as MCP clients and CLIs, send those from any origin. A cross-origin page cannot add such a
// header to a request that carries cookies: the header requires a CORS preflight, and Obot's CORS policy never allows
// credentials. A request without any of these cookies has no user's authority to abuse, and anonymous
// endpoints, such as OAuth token exchange, are called cross-origin by browser-based clients.
func (s *Server) rejectCrossOrigin(req *http.Request) bool {
	// CrossOriginProtection allows these methods, so their cookies need not be read.
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	if !cookieAuthenticated(req) {
		return false
	}

	return s.crossOrigin.reject(req)
}

// cookieAuthenticated reports whether req can be authenticated only by a browser cookie: it carries Obot's session or
// bootstrap cookie and no header credential.
func cookieAuthenticated(req *http.Request) bool {
	if hasHeaderCredential(req) {
		return false
	}
	for _, name := range obotCookies {
		if _, err := req.Cookie(name); err == nil {
			return true
		}
	}
	return false
}

// hasHeaderCredential reports whether req carries a credential in a header that a client sets itself. An Authorization
// header with a scheme other than Bearer, such as Basic, is not one: a browser attaches it on its own from cached HTTP
// authentication.
//
// The other credential headers count wherever they appear, even on a path that does not authenticate them, such as
// X-Obot-Api-Key outside the LLM proxy. What makes the exemption safe is not that the header authenticates the request,
// but that a cross-origin page cannot send it with cookies at all (see rejectCrossOrigin).
func hasHeaderCredential(req *http.Request) bool {
	const bearerPrefix = "Bearer "
	for _, header := range auth.CredentialHeaders {
		value := req.Header.Get(header)
		if value == "" {
			continue
		}
		if header == "Authorization" && (len(value) <= len(bearerPrefix) || !strings.EqualFold(value[:len(bearerPrefix)], bearerPrefix)) {
			continue
		}
		return true
	}
	return false
}
