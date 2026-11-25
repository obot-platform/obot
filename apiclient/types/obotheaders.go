package types

import (
	"fmt"
	"slices"
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

	// ObotHeaderOnBehalfOfUserID, ObotHeaderOnBehalfOfUserName and ObotHeaderOnBehalfOfUserEmail carry the user an
	// agent acts for, beside the agent itself in ObotHeaderUserID, ObotHeaderUserName and ObotHeaderUserEmail. They are
	// not selected on their own: a backend that selects one of those three gets its on-behalf-of counterpart with it
	// (OnBehalfOfObotHeader), whenever the caller is an agent acting on behalf of a user.
	ObotHeaderOnBehalfOfUserID    ObotHeader = "X-Obot-On-Behalf-Of-User-Id"
	ObotHeaderOnBehalfOfUserName  ObotHeader = "X-Obot-On-Behalf-Of-User-Name"
	ObotHeaderOnBehalfOfUserEmail ObotHeader = "X-Obot-On-Behalf-Of-User-Email"
)

// ObotHeader is an optional, gateway-derived attribution header sent to an MCP backend.
type ObotHeader string

// ObotHeaders returns the supported attribution headers in display order.
func ObotHeaders() []ObotHeader {
	return []ObotHeader{ObotHeaderAPIKeyName, ObotHeaderAPIKeyID, ObotHeaderUserID, ObotHeaderUserName, ObotHeaderUserEmail, ObotHeaderIDPUserID, ObotHeaderIDPGroups}
}

// OnBehalfOfObotHeader returns the header that carries the on-behalf-of counterpart of the selectable header, and
// false for a header that has none.
func OnBehalfOfObotHeader(header ObotHeader) (ObotHeader, bool) {
	switch header {
	case ObotHeaderUserID:
		return ObotHeaderOnBehalfOfUserID, true
	case ObotHeaderUserName:
		return ObotHeaderOnBehalfOfUserName, true
	case ObotHeaderUserEmail:
		return ObotHeaderOnBehalfOfUserEmail, true
	}
	return "", false
}

// ReservedObotHeaders returns every attribution header Obot derives: the selectable ones (ObotHeaders), then their
// on-behalf-of counterparts. A client never supplies any of them, and no configured header may use their names.
func ReservedObotHeaders() []ObotHeader {
	return append(ObotHeaders(), ObotHeaderOnBehalfOfUserID, ObotHeaderOnBehalfOfUserName, ObotHeaderOnBehalfOfUserEmail)
}

// ReservedObotHeader recognizes every reserved attribution header name (ReservedObotHeaders), in canonical case.
func ReservedObotHeader(name string) (ObotHeader, bool) {
	for _, header := range ReservedObotHeaders() {
		if strings.EqualFold(name, string(header)) {
			return header, true
		}
	}
	return "", false
}

// SupportsObotHeaders reports whether a runtime supports HTTP caller attribution.
func SupportsObotHeaders(runtime Runtime) bool {
	return runtime == RuntimeRemote || runtime == RuntimeContainerized
}

// CanonicalObotHeader recognizes only the selectable attribution header names (ObotHeaders). ReservedObotHeader also
// recognizes their on-behalf-of counterparts.
func CanonicalObotHeader(name string) (ObotHeader, bool) {
	for _, header := range ObotHeaders() {
		if strings.EqualFold(name, string(header)) {
			return header, true
		}
	}
	return "", false
}

// NormalizeObotHeaders returns a copy with supported names in canonical case.
// Unknown names and duplicates remain so validation can reject them.
func NormalizeObotHeaders(headers []ObotHeader) []ObotHeader {
	result := slices.Clone(headers)
	for i, name := range result {
		if header, ok := CanonicalObotHeader(string(name)); ok {
			result[i] = header
		}
	}
	return result
}

// ValidateObotHeaders validates attribution selections without modifying the caller's slice.
func ValidateObotHeaders(runtime Runtime, headers []ObotHeader) error {
	if len(headers) > 0 && !SupportsObotHeaders(runtime) {
		return fmt.Errorf("injectObotHeaders is only supported for remote and containerized MCP servers")
	}
	seen := make(map[ObotHeader]struct{}, len(headers))
	for _, name := range headers {
		header, ok := CanonicalObotHeader(string(name))
		if !ok {
			return fmt.Errorf("unsupported injectObotHeaders header %q", name)
		}
		if _, exists := seen[header]; exists {
			return fmt.Errorf("duplicate injectObotHeaders header %q", name)
		}
		seen[header] = struct{}{}
	}
	return nil
}
