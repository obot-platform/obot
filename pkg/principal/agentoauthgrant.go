package principal

import (
	"strings"
)

const (
	// agentOAuthGrantPrefix starts the owner of a vMCP's agent OAuth grants (AgentOAuthGrantOwner).
	agentOAuthGrantPrefix = "vmcp-agent-oauth:"
)

// AgentOAuthGrantOwner returns the owner that the OAuth grants an administrator authorizes on a shared vMCP for agents
// are stored under, in the UserID of their MCP OAuth tokens: "vmcp-agent-oauth:<vmcpID>". The vMCP holds one grant per
// component, and every agent that a profile of the vMCP grants the component uses it.
//
// The owner is not a principal. Parse refuses it, so FromUserInfo never reads a caller with it as a user or an agent,
// no authenticator builds one, and the admission check refuses any caller that carries it (IsAgentOAuthGrantOwner).
// Nothing can sign in as a vMCP's grants, and no subject, policy or cleanup of a user or an agent ever names them.
func AgentOAuthGrantOwner(vmcpID string) string {
	return agentOAuthGrantPrefix + vmcpID
}

// ParseAgentOAuthGrantOwner returns the vMCP of an agent OAuth grant owner (AgentOAuthGrantOwner). It reports false
// for anything else, including a malformed owner with the prefix.
func ParseAgentOAuthGrantOwner(owner string) (vmcpID string, ok bool) {
	vmcpID, ok = strings.CutPrefix(owner, agentOAuthGrantPrefix)
	if !ok || vmcpID == "" || strings.Contains(vmcpID, "/") {
		return "", false
	}
	return vmcpID, true
}

// IsAgentOAuthGrantOwner reports whether owner has the prefix of an agent OAuth grant owner, whether or not it is well
// formed. Code that must never act for a vMCP's grants, such as authentication, refuses any such owner.
func IsAgentOAuthGrantOwner(owner string) bool {
	return strings.HasPrefix(owner, agentOAuthGrantPrefix)
}
