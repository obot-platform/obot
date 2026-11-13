package principal

import (
	"strings"
)

const (
	// profileGrantPrefix starts the owner of a vMCP profile's OAuth grants (ProfileGrant).
	profileGrantPrefix = "vmcp-profile:"
)

// ProfileGrant returns the owner that the OAuth grants an administrator authorizes on a vMCP profile are stored
// under, in the UserID of their MCP OAuth tokens: "vmcp-profile:<vmcpID>/<profileID>". Every agent that resolves to
// the profile uses them.
//
// The owner is not a principal. Parse refuses it, so FromUserInfo never reads a caller with it as a user or an agent,
// no authenticator builds one, and the admission check refuses any caller that carries it (IsProfileGrant). Nothing
// can sign in as a profile, and no subject, policy or cleanup of a user or an agent ever names one.
func ProfileGrant(vmcpID, profileID string) string {
	return profileGrantPrefix + vmcpID + "/" + profileID
}

// ParseProfileGrant returns the vMCP and profile of a profile grant owner (ProfileGrant). It reports false for
// anything else, including a malformed owner with the profile grant prefix.
func ParseProfileGrant(owner string) (vmcpID, profileID string, ok bool) {
	rest, ok := strings.CutPrefix(owner, profileGrantPrefix)
	if !ok {
		return "", "", false
	}
	vmcpID, profileID, ok = strings.Cut(rest, "/")
	if !ok || vmcpID == "" || profileID == "" || strings.Contains(profileID, "/") {
		return "", "", false
	}
	return vmcpID, profileID, true
}

// IsProfileGrant reports whether owner has the prefix of a profile grant owner, whether or not it is well formed. Code
// that must never act for a profile grant, such as authentication, refuses any such owner.
func IsProfileGrant(owner string) bool {
	return strings.HasPrefix(owner, profileGrantPrefix)
}
