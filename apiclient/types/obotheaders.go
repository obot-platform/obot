package types

import (
	"fmt"
	"strings"
)

const (
	ObotHeaderAPIKeyName ObotHeader = "X-Obot-Api-Key-Name"
	ObotHeaderAPIKeyID   ObotHeader = "X-Obot-Api-Key-Id"
	ObotHeaderUserID     ObotHeader = "X-Obot-User-Id"
	ObotHeaderUserName   ObotHeader = "X-Obot-User-Name"
	ObotHeaderUserEmail  ObotHeader = "X-Obot-User-Email"
	ObotHeaderIDPUserID  ObotHeader = "X-Obot-IDP-User-Id"
	ObotHeaderIDPGroups  ObotHeader = "X-Obot-IDP-Groups"
)

// ObotHeader is an optional, gateway-derived attribution header sent to an MCP backend.
type ObotHeader string

// ObotHeaders returns the supported attribution headers in display order.
func ObotHeaders() []ObotHeader {
	return []ObotHeader{ObotHeaderAPIKeyName, ObotHeaderAPIKeyID, ObotHeaderUserID, ObotHeaderUserName, ObotHeaderUserEmail, ObotHeaderIDPUserID, ObotHeaderIDPGroups}
}

// SupportsObotHeaders reports whether a runtime supports HTTP caller attribution.
func SupportsObotHeaders(runtime Runtime) bool {
	return runtime == RuntimeRemote
}

// CanonicalObotHeader recognizes only the reserved attribution header names.
func CanonicalObotHeader(name string) (ObotHeader, bool) {
	for _, header := range ObotHeaders() {
		if strings.EqualFold(name, string(header)) {
			return header, true
		}
	}
	return "", false
}

// ValidateObotHeaders validates attribution selections without modifying the caller's slice.
func ValidateObotHeaders(runtime Runtime, headers []ObotHeader) error {
	if len(headers) > 0 && !SupportsObotHeaders(runtime) {
		return fmt.Errorf("injectObotHeaders is only supported for remote MCP servers")
	}
	seen := make(map[ObotHeader]bool, len(headers))
	for _, name := range headers {
		header, ok := CanonicalObotHeader(string(name))
		if !ok {
			return fmt.Errorf("unsupported injectObotHeaders header %q", name)
		}
		if seen[header] {
			return fmt.Errorf("duplicate injectObotHeaders header %q", name)
		}
		seen[header] = true
	}
	return nil
}
