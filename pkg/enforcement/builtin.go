package enforcement

import (
	"strings"
)

// builtinAgentMCPServers is the explicit, per-agent set of MCP server names that
// ship inside the agent itself (as opposed to servers a user configures). A call
// to one of these is what the "Allow all built-in agent MCP servers" coarse
// toggle permits.
//
// Claude Code's names are sound to match on alone: it publishes a hardcoded
// list and will not let a configured server take one of those names, so a
// built-in name can never also be a configuration key on the device side.
//
// Codex has one: node_repl, the JavaScript REPL that Codex Desktop's built-in
// browser control runs on. Unlike Claude Code's, it is an ordinary
// [mcp_servers.node_repl] entry that Codex Desktop writes into
// ~/.codex/config.toml, so a user could define a server of the same name. The
// device only reports it as built-in — by name, with no server identity — when
// the entry's command is the executable shipped inside the Codex app, and
// isBuiltinAgentMCPCall requires exactly that shape. Codex's computer-use is not
// a built-in: it is installed by the ChatGPT app, not shipped with Codex.
//
// This table must agree with the copy in obot-sentry's pkg/enforce, which decides
// which calls are reported as built-in servers at all. Membership is intentionally
// kept out of any user-facing copy, and the evaluator's behavior does not depend
// on the exact contents of this map.
var (
	builtinAgentMCPServers = map[string]map[string]struct{}{
		AgentClaudeCode: {
			"workspace":        {},
			"claude-in-chrome": {},
			"computer-use":     {},
			"Claude Preview":   {},
			"Claude Browser":   {},
		},
		AgentCodex: {
			"node_repl": {},
		},
	}

	builtinAgentToolKinds = map[string]struct{}{
		KindShell:   {},
		KindRead:    {},
		KindWrite:   {},
		KindTask:    {},
		KindGeneric: {},
	}
)

func isBuiltinAgentToolKind(kind string) bool {
	_, ok := builtinAgentToolKinds[kind]
	return ok
}

// isBuiltinAgentMCPCall reports whether call is to a built-in agent MCP server.
//
// A built-in is reported by the device by name alone: it appears in no
// configuration the device could reduce to a URL, package, command, or
// connector. A call that does carry an identity was resolved from a configured
// entry, and a configured server that merely shares a built-in's name must be
// matched against the allowlist like any other, not waved through on its name.
func isBuiltinAgentMCPCall(call NormalizedCall) bool {
	if call.Server.URL != "" || call.Server.Package != nil || call.Server.Command != "" ||
		call.Server.Hostname != "" || call.Server.Connector != "" {
		return false
	}
	return isBuiltinAgentMCP(call.Agent, call.ServerName)
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
