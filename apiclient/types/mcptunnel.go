package types

import (
	"fmt"
	"net/url"
	"strings"
)

// MCPTunnel is an admin-managed tunnel configuration.
//
// Token contains the complete bearer token only when the tunnel is created or
// its token is rotated. Other responses contain a non-secret preview.
type MCPTunnel struct {
	Metadata
	Manifest MCPTunnelManifest `json:"manifest"`
	Token    string            `json:"token"`
}

type MCPTunnelManifest struct {
	DisplayName string   `json:"displayName"`
	Description string   `json:"description,omitempty"`
	AllowedURLs []string `json:"allowedURLs,omitempty"`
}

type MCPTunnelList List[MCPTunnel]

// Validate checks the user-managed fields of an MCP tunnel manifest.
func (m MCPTunnelManifest) Validate() error {
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("displayName is required")
	}

	for i, allowedURL := range m.AllowedURLs {
		allowedURL = strings.TrimSpace(allowedURL)
		if allowedURL == "" {
			return fmt.Errorf("allowedURLs[%d] must not be empty", i)
		}

		switch strings.Count(allowedURL, "*") {
		case 0:
		case 1:
			if !strings.HasPrefix(allowedURL, "*") && !strings.HasSuffix(allowedURL, "*") {
				return fmt.Errorf("allowedURLs[%d] wildcard must be at the beginning or end", i)
			}
		default:
			return fmt.Errorf("allowedURLs[%d] must contain at most one wildcard", i)
		}
	}

	return nil
}

// AllowsURL reports whether target matches an exact, prefix, or suffix pattern
// in AllowedURLs. For ordinary HTTP(S) URLs, patterns are also checked against
// the parsed hostname so entries such as "api.internal" and "*.internal" can
// allow a complete target URL.
func (m MCPTunnelManifest) AllowsURL(target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}

	var hostname string
	normalizedTarget := target
	if parsed, err := url.Parse(target); err == nil &&
		(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		hostname = parsed.Hostname()
		if normalized, ok := normalizeMCPTunnelURL(target); ok {
			normalizedTarget = normalized
		}
	}

	for _, allowedURL := range m.AllowedURLs {
		allowedURL = strings.TrimSpace(allowedURL)
		if isMCPTunnelURLPattern(allowedURL) {
			normalizedAllowedURL := allowedURL
			if normalized, ok := normalizeMCPTunnelURLPattern(allowedURL); ok {
				normalizedAllowedURL = normalized
			}
			if matchesMCPTunnelAllowedURL(normalizedAllowedURL, normalizedTarget) {
				return true
			}
			continue
		}

		hostnameTarget := target
		if hostname != "" {
			hostnameTarget = hostname
		}
		if matchesMCPTunnelAllowedURL(strings.ToLower(allowedURL), strings.ToLower(hostnameTarget)) {
			return true
		}
	}

	return false
}

func matchesMCPTunnelAllowedURL(pattern, target string) bool {
	switch {
	case pattern == "":
		return false
	case pattern == "*":
		return true
	case strings.HasPrefix(pattern, "*"):
		return strings.HasSuffix(target, strings.TrimPrefix(pattern, "*"))
	case strings.HasSuffix(pattern, "*"):
		return strings.HasPrefix(target, strings.TrimSuffix(pattern, "*"))
	default:
		return target == pattern
	}
}

func isMCPTunnelURLPattern(pattern string) bool {
	candidate := strings.TrimPrefix(strings.TrimSuffix(pattern, "*"), "*")
	lowerCandidate := strings.ToLower(candidate)
	return strings.HasPrefix(lowerCandidate, "http:") ||
		strings.HasPrefix(lowerCandidate, "https:") ||
		strings.ContainsAny(candidate, "/?#")
}

func normalizeMCPTunnelURLPattern(pattern string) (string, bool) {
	pattern, hasLeadingWildcard := strings.CutPrefix(pattern, "*")
	pattern, hasTrailingWildcard := strings.CutSuffix(pattern, "*")
	normalized, ok := normalizeMCPTunnelURL(pattern)
	if !ok {
		return "", false
	}
	if hasLeadingWildcard {
		normalized = "*" + normalized
	}
	if hasTrailingWildcard {
		normalized += "*"
	}
	return normalized, true
}

func normalizeMCPTunnelURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" ||
		(!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return "", false
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") ||
		(parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	if port != "" {
		hostname += ":" + port
	}
	parsed.Host = hostname
	return parsed.String(), true
}
