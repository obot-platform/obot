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
)

// ObotHeader is an optional, gateway-derived attribution header sent to an MCP backend.
type ObotHeader string

// CanonicalObotHeader recognizes only the reserved attribution header names.
func CanonicalObotHeader(name string) (ObotHeader, bool) {
	for _, header := range []ObotHeader{ObotHeaderAPIKeyName, ObotHeaderAPIKeyID, ObotHeaderUserID, ObotHeaderUserName, ObotHeaderUserEmail} {
		if strings.EqualFold(name, string(header)) {
			return header, true
		}
	}
	return "", false
}

// NormalizeObotHeaders validates and canonicalizes the selected attribution headers.
func NormalizeObotHeaders(runtime Runtime, headers []ObotHeader) error {
	if len(headers) > 0 && runtime != RuntimeRemote && runtime != RuntimeContainerized {
		return fmt.Errorf("injectObotHeaders is only supported for remote and containerized MCP servers")
	}
	seen := make(map[ObotHeader]bool, len(headers))
	for i, name := range headers {
		header, ok := CanonicalObotHeader(string(name))
		if !ok {
			return fmt.Errorf("unsupported injectObotHeaders header %q", name)
		}
		if seen[header] {
			return fmt.Errorf("duplicate injectObotHeaders header %q", name)
		}
		seen[header] = true
		headers[i] = header
	}
	return nil
}
