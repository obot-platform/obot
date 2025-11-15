package registry

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var dnsLabelRegex = regexp.MustCompile("[^a-z0-9-]+")

// ReverseDNSFromURL converts a URL like "https://obot.example.com" to "com.example.obot"
// Handles localhost and IP addresses specially by returning "local.<hostname>"
func ReverseDNSFromURL(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("base URL has no hostname")
	}

	// Handle localhost and loopback addresses
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		return "local.localhost", nil
	}

	// Handle other IP addresses
	if isIPAddress(hostname) {
		// Convert IP dots to hyphens for DNS label compliance
		normalized := strings.ReplaceAll(hostname, ".", "-")
		return fmt.Sprintf("local.%s", normalized), nil
	}

	// Split hostname into parts
	parts := strings.Split(hostname, ".")

	// Reverse the parts
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}

	// Normalize each label to be DNS-compliant
	normalizedParts := make([]string, len(parts))
	for i, part := range parts {
		normalizedParts[i] = normalizeDNSLabel(part)
	}

	return strings.Join(normalizedParts, "."), nil
}

// normalizeDNSLabel ensures a string is a valid DNS label (lowercase, alphanumeric + hyphens, no leading/trailing hyphens)
func normalizeDNSLabel(label string) string {
	// Convert to lowercase
	label = strings.ToLower(label)

	// Replace invalid characters with hyphens
	label = dnsLabelRegex.ReplaceAllString(label, "-")

	// Collapse multiple hyphens
	for strings.Contains(label, "--") {
		label = strings.ReplaceAll(label, "--", "-")
	}

	// Trim leading/trailing hyphens
	label = strings.Trim(label, "-")

	// Enforce max length 63 (DNS label limit)
	if len(label) > 63 {
		label = label[:63]
		label = strings.TrimRight(label, "-")
	}

	return label
}

// isIPAddress checks if a hostname is an IP address
func isIPAddress(hostname string) bool {
	// Simple check: contains only digits, dots, and colons (IPv4 or IPv6)
	for _, char := range hostname {
		if char != '.' && char != ':' && (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return strings.Contains(hostname, ".") || strings.Contains(hostname, ":")
}

// FormatRegistryServerName creates a full registry server name from reverse DNS and server name
// Example: "com.example.obot/my-server"
func FormatRegistryServerName(reverseDNS, serverName string) string {
	// Normalize server name to be DNS-compliant
	normalizedName := normalizeDNSLabel(serverName)
	return fmt.Sprintf("%s/%s", reverseDNS, normalizedName)
}
