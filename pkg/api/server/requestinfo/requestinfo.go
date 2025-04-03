package requestinfo

import (
	"net/http"
	"strings"
)

// GetSourceIP extracts the real client IP address from the request.
// It checks X-Forwarded-For and X-Real-IP headers before falling back to RemoteAddr.
func GetSourceIP(req *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs (client, proxy1, proxy2, ...).
		// A typical deployment of Obot will have a proxy in front of it that appends the request client's IP address (req.RemoteAddr) to the X-Forwarded-For header before forwarding the request to Obot.
		// With that in mind, we choose the rightmost IP in the X-Forwarded-For header since it's the only IP address that is not spoofable by the client.
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[len(ips)-1])
	}

	// Check X-Real-IP header next
	if xrip := req.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Fall back to RemoteAddr
	return req.RemoteAddr
}
