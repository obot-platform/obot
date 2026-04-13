package handlers

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// NormalizeOrigin validates and normalizes an origin URL and returns its hostname for comparisons.
func NormalizeOrigin(raw string) (origin, hostname string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}

	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("must include scheme and host")
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", "", fmt.Errorf("must not include path, query, or fragment")
	}

	return u.String(), strings.ToLower(u.Hostname()), nil
}

// HostnameMatches compares hostnames after removing ports, brackets, and casing differences.
func HostnameMatches(host, hostname string) bool {
	host = normalizedHostname(host)
	hostname = normalizedHostname(hostname)
	return host != "" && hostname != "" && host == hostname
}

func normalizedHostname(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}
