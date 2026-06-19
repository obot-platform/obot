package oauth

import (
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

func isRedirectURIAllowed(manifest types.OAuthClientManifest, redirectURI string) bool {
	if slices.Contains(manifest.RedirectURIs, redirectURI) {
		return true
	}
	if manifest.ApplicationType != "native" {
		return false
	}

	for _, registeredRedirectURI := range manifest.RedirectURIs {
		if isNativeLoopbackRedirectURIWithRequestedPort(registeredRedirectURI, redirectURI) {
			return true
		}
	}

	return false
}

func isNativeLoopbackRedirectURIWithRequestedPort(registeredRedirectURI, requestedRedirectURI string) bool {
	registered, err := url.Parse(registeredRedirectURI)
	if err != nil {
		return false
	}
	requested, err := url.Parse(requestedRedirectURI)
	if err != nil {
		return false
	}
	if registered.Port() != "" || requested.Port() == "" {
		return false
	}
	registeredHost := registered.Hostname()
	requestedHost := requested.Hostname()
	if !isLoopbackHost(registeredHost) || !strings.EqualFold(registeredHost, requestedHost) {
		return false
	}

	return registered.Scheme == requested.Scheme &&
		registered.User == nil &&
		requested.User == nil &&
		registered.EscapedPath() == requested.EscapedPath() &&
		registered.RawQuery == requested.RawQuery &&
		registered.Fragment == requested.Fragment
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}
