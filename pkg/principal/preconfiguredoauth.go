package principal

import (
	"strings"
)

const (
	// preconfiguredOAuthPrefix starts the owner of a vMCP's preconfigured OAuth grants (PreconfiguredOAuthOwner).
	preconfiguredOAuthPrefix = "vmcp-oauth:"
)

// PreconfiguredOAuthOwner returns the owner that the OAuth grants an administrator preconfigures on a shared vMCP are
// stored under, in the UserID of their MCP OAuth tokens: "vmcp-oauth:<vmcpID>". The vMCP holds at most one grant per
// remote component, and everyone who reaches the component through the vMCP, users and agents alike, uses it.
//
// The owner is not a principal. Parse refuses it, so FromUserInfo never reads a caller with it as a user or an agent,
// no authenticator builds one, and the admission check refuses any caller that carries it (IsPreconfiguredOAuthOwner).
// Nothing can sign in as a vMCP's grants, and no subject, policy or cleanup of a user or an agent ever names them.
func PreconfiguredOAuthOwner(vmcpID string) string {
	return preconfiguredOAuthPrefix + vmcpID
}

// ParsePreconfiguredOAuthOwner returns the vMCP of a preconfigured OAuth owner (PreconfiguredOAuthOwner). It reports
// false for anything else, including a malformed owner with the prefix.
func ParsePreconfiguredOAuthOwner(owner string) (vmcpID string, ok bool) {
	vmcpID, ok = strings.CutPrefix(owner, preconfiguredOAuthPrefix)
	if !ok || vmcpID == "" || strings.Contains(vmcpID, "/") {
		return "", false
	}
	return vmcpID, true
}

// IsPreconfiguredOAuthOwner reports whether owner has the prefix of a preconfigured OAuth owner, whether or not it is
// well formed. Code that must never act for a vMCP's grants, such as authentication, refuses any such owner.
func IsPreconfiguredOAuthOwner(owner string) bool {
	return strings.HasPrefix(owner, preconfiguredOAuthPrefix)
}
