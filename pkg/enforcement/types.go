// Package enforcement contains the pure decision logic that decides whether a
// normalized tool call is permitted by an MDMConfiguration's EnforcementAllowlist.
//
// The evaluator is deliberately I/O-free: it takes a NormalizedCall plus an
// allowlist and returns an allow/deny Decision. The decision endpoint and the
// device-side hook both build a NormalizedCall and feed it here.
package enforcement

import "github.com/obot-platform/obot/apiclient/types"

// Tool-call kinds produced by the device-side tool classifier. These mirror the
// classifications obot-sentry derives from a runtime tool name.
const (
	KindMCP     = "mcp"
	KindShell   = "shell"
	KindRead    = "read"
	KindWrite   = "write"
	KindTask    = "task"
	KindGeneric = "generic"
)

// Supported local agents.
const (
	AgentClaudeCode = "claude_code"
	AgentCodex      = "codex"
	AgentVSCode     = "vscode"
	AgentCursor     = "cursor"
)

// NormalizedCall is the parameter-free description of a single tool call that
// the evaluator decides on. It is produced by the device-side pre-tool hook
// (resolving the target server from the agent's MCP config) and by the decision
// endpoint before evaluation.
type NormalizedCall struct {
	// Agent is the coding agent that issued the call (claude_code | codex | vscode | cursor).
	Agent string
	// Tool is the runtime tool name (for an MCP call this is the tool within the server).
	Tool string
	// Kind is the classified tool kind: mcp | shell | read | write | task | generic.
	Kind string
	// ServerName is the MCP server hint derived from the tool name (e.g. the
	// "<server>" in mcp__<server>__<tool>). It may be empty when the agent does
	// not expose a server hint. It is used to match the built-in agent MCP set.
	ServerName string
	// Server identifies the resolved target MCP server (for mcp calls).
	Server ServerIdentity
	// ObotHosted is true when the resolved server maps to an Obot-hosted or
	// system MCP server.
	ObotHosted bool
}

// ServerIdentity identifies a resolved MCP server. For an MCP call at most one
// of URL / Package / Command is populated; Hostname is derived from URL when
// present.
type ServerIdentity struct {
	URL      string
	Package  *PackageIdentity
	Command  string
	Hostname string
}

// PackageIdentity is the canonical package identity of a stdio MCP server that
// is launched via a package runner (npx / uvx).
type PackageIdentity struct {
	Source  types.AllowlistServerPackageSource
	Name    string
	Version string
}

// Decision is the result of evaluating a NormalizedCall against an allowlist.
type Decision struct {
	Allow  bool
	Reason string
}
