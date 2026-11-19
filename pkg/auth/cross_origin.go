package auth

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const (
	// HeaderObotAPIKey carries an Obot API key alongside another credential, on LLM proxy requests. It is
	// authenticated only there.
	HeaderObotAPIKey = "X-Obot-Api-Key"
)

var (
	// CredentialHeaders are the request headers from which Obot's authenticators read a credential, such as a bearer
	// token or an API key, that a client sets itself. They do not include the headers of upstream credentials, such as
	// an LLM provider's API key, that Obot forwards or strips but never authenticates.
	CredentialHeaders = []string{"Authorization", "X-API-Key", HeaderObotAPIKey}
)

// TrustOrigins adds the origins of Obot's own URLs to protection, so that requests from them are not refused as
// cross-origin. A URL without a scheme and host is skipped with a warning.
func TrustOrigins(protection *http.CrossOriginProtection, trustedURLs ...string) {
	for _, origin := range TrustedOrigins(trustedURLs...) {
		if err := protection.AddTrustedOrigin(origin); err != nil {
			slog.Warn("Failed to trust a URL for cross-origin requests", "origin", origin, "error", err)
		}
	}
}

// TrustedOrigins returns the origins of Obot's own URLs, in the scheme://host form that browsers send in Origin. A URL
// without a scheme and host is skipped with a warning, so the result never contains an empty origin.
func TrustedOrigins(trustedURLs ...string) []string {
	origins := make([]string, 0, len(trustedURLs))
	for _, trusted := range trustedURLs {
		origin := OriginOf(trusted)
		if origin == "" {
			if trusted != "" {
				slog.Warn("Not trusting an invalid URL for cross-origin requests", "url", trusted)
			}
			continue
		}
		origins = append(origins, origin)
	}
	return origins
}

// OriginOf returns the scheme://host origin of rawURL, such as a Referer, or "" if it has no scheme and host.
//
// The origin is in the canonical form that browsers send in Origin and Referer: a lowercase host, without the scheme's
// default port. Trusted origins are compared with what a browser sends as exact strings, both here and in
// http.CrossOriginProtection, so a configured URL such as https://Obot.Example.com:443 would otherwise never match.
func OriginOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	// url.Parse already lowercases the scheme, but keeps the host as written.
	host := strings.ToLower(u.Host)
	// Trimming the suffix, rather than splitting the port off, keeps the brackets of an IPv6 host such as [::1]:443.
	switch u.Scheme {
	case "http":
		host = strings.TrimSuffix(host, ":80")
	case "https":
		host = strings.TrimSuffix(host, ":443")
	}
	return u.Scheme + "://" + host
}
