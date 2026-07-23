package enforcement

// builtinAgentMCPServers is the explicit, per-agent set of MCP server names that
// ship inside the agent itself (as opposed to servers a user configures). A call
// to one of these is what the "Allow all built-in agent MCP servers" coarse
// toggle permits.
//
// Membership is intentionally kept out of any user-facing copy and is expected
// to be curated over time as agents add or rename their built-in servers. The
// evaluator's behavior does not depend on the exact contents of this map.
var builtinAgentMCPServers = map[string]map[string]struct{}{
	AgentCodex: {
		// Codex's built-in computer-use MCP server.
		"computer-use": {},
	},
}

// isBuiltinAgentMCP reports whether serverName is a built-in MCP server for the
// given agent.
func isBuiltinAgentMCP(agent, serverName string) bool {
	if serverName == "" {
		return false
	}
	servers, ok := builtinAgentMCPServers[agent]
	if !ok {
		return false
	}
	_, ok = servers[serverName]
	return ok
}
