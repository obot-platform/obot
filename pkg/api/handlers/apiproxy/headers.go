package apiproxy

import (
	"net/http"
	"strings"
)

type HeaderPolicy struct {
	Name        string
	Forwardable bool
	Sensitive   bool
	Credential  bool
}

func SanitizeInputHeaders(input http.Header, policies []HeaderPolicy) http.Header {
	result := http.Header{}
	for name, values := range input {
		if isProtectedInputHeader(name) || !headerForwardable(name, policies) {
			continue
		}
		result[name] = append([]string(nil), values...)
	}
	return result
}

func WriteBusinessHeaders(target http.Header, source []HeaderValue) {
	for _, header := range source {
		if isHopByHopHeader(header.Name) || strings.EqualFold(header.Name, "Content-Length") ||
			strings.HasPrefix(strings.ToLower(strings.TrimSpace(header.Name)), "x-obot-") {
			continue
		}
		for _, value := range header.Values {
			target.Add(header.Name, value)
		}
	}
}

func headerForwardable(name string, policies []HeaderPolicy) bool {
	for _, policy := range policies {
		if strings.EqualFold(policy.Name, name) {
			return policy.Forwardable && !policy.Sensitive && !policy.Credential
		}
	}
	return false
}

func isProtectedInputHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "x-api-key", "x-obot-api-key", "x-obot-worker-authentication",
		"cookie", "set-cookie", "host", "content-length", "transfer-encoding", "connection",
		"keep-alive", "proxy-authenticate", "proxy-authorization", "proxy-connection",
		"te", "trailer", "upgrade", "via", "forwarded":
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(lower, "x-obot-") || strings.HasPrefix(lower, "x-forwarded-")
}

func isHopByHopHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"proxy-connection", "te", "trailer", "transfer-encoding", "upgrade":
		return true
	default:
		return false
	}
}
