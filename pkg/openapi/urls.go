package openapi

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode"
)

// sourceURL checks the schema location; safehttp enforces its network policy.
func sourceURL(value string) (*url.URL, error) {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || strings.ContainsAny(value, "\\\r\n\t") {
		return nil, fmt.Errorf("schema source must be an absolute HTTP(S) URL without credentials or fragment")
	}
	return u, nil
}

// destination checks the API base URL against the hosted wrapper contract.
// The wrapper enforces DNS/address policy again when executing API requests.
func destination(value string) (string, error) {
	u, err := sourceURL(value)
	if err != nil || u.RawQuery != "" || u.ForceQuery || strings.ContainsAny(value, "{}") || strings.ContainsFunc(value, unicode.IsSpace) {
		return "", fmt.Errorf("baseURL must be absolute HTTP(S), without credentials, query, fragment, or variables")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return "", fmt.Errorf("local API destinations are unsupported")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" || netip.MustParsePrefix("0.0.0.0/8").Contains(ip) {
			return "", fmt.Errorf("local, link-local, unspecified, and multicast API destinations are unsupported")
		}
	}
	return strings.TrimRight(u.String(), "/") + "/", nil
}
