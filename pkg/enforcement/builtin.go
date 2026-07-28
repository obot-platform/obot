package enforcement

import "strings"

// builtinAgentMCPServers is the explicit, per-agent set of MCP server names that
// ship inside the agent itself (as opposed to servers a user configures). A call
// to one of these is what the "Allow all built-in agent MCP servers" coarse
// toggle permits.
//
// Claude Code is the only agent here, and it is the only one where name-based
// matching is sound: it publishes a hardcoded list and will not let a configured
// server take one of those names, so a built-in name can never also be a
// configuration key on the device side.
//
// Codex is deliberately absent. Its computer-use server is not a built-in: it is
// an ordinary [mcp_servers.computer-use] entry in ~/.codex/config.toml installed
// by the ChatGPT app, with a local executable for a command. Neither a published
// list nor a collision guarantee exists for Codex, so there is nothing to base a
// name match on.
//
// This table must agree with the copy in obot-sentry's pkg/enforce, which decides
// which calls are reported as built-in servers at all. Membership is intentionally
// kept out of any user-facing copy, and the evaluator's behavior does not depend
// on the exact contents of this map.
var builtinAgentMCPServers = map[string]map[string]struct{}{
	AgentClaudeCode: {
		"workspace":        {},
		"claude-in-chrome": {},
		"computer-use":     {},
		"Claude Preview":   {},
		"Claude Browser":   {},
	},
}

var builtinAgentToolKinds = map[string]struct{}{
	KindShell:   {},
	KindRead:    {},
	KindWrite:   {},
	KindTask:    {},
	KindGeneric: {},
}

func isBuiltinAgentToolKind(kind string) bool {
	_, ok := builtinAgentToolKinds[kind]
	return ok
}

// isBuiltinAgentMCP reports whether serverName is a built-in MCP server for the
// given agent, matching exactly first and then by normalized name.
//
// The normalized rung is required: two of Claude Code's five built-in names
// contain spaces and capitals, and no tool namespace preserves either, so
// "Claude Preview" reaches a device-side hook as something like "Claude_Preview"
// and an exact-only comparison would miss it every time. The device applies the
// same ladder.
func isBuiltinAgentMCP(agent, serverName string) bool {
	if serverName == "" {
		return false
	}
	servers, ok := builtinAgentMCPServers[agent]
	if !ok {
		return false
	}
	if _, ok := servers[serverName]; ok {
		return true
	}
	target := normalizedServerName(serverName)
	if target == "" {
		return false
	}
	for name := range servers {
		if normalizedServerName(name) == target {
			return true
		}
	}
	return false
}

// normalizedServerName reduces a server name to lowercase alphanumerics, which is
// what makes a built-in name comparable to the form a tool namespace carries.
func normalizedServerName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
