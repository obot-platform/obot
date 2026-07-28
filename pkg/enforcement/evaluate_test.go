package enforcement

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func npmPkg(name, version string) *PackageIdentity {
	return &PackageIdentity{Source: types.AllowlistServerPackageSourceNPM, Name: name, Version: version}
}

func pypiPkg(name, version string) *PackageIdentity {
	return &PackageIdentity{Source: types.AllowlistServerPackageSourcePyPI, Name: name, Version: version}
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name      string
		call      NormalizedCall
		allowlist types.EnforcementAllowlist
		wantAllow bool
	}{
		// Allow everything short-circuit.
		{
			name:      "allow everything short-circuits even for unresolved mcp call",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "anything"},
			allowlist: types.EnforcementAllowlist{AllowEverything: true},
			wantAllow: true,
		},

		// Coarse: built-in agent tools = non-MCP kinds.
		{
			name:      "builtin agent tools allows shell",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindShell, Tool: "bash"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true},
			wantAllow: true,
		},
		{
			name:      "builtin agent tools allows read/write/task/generic",
			call:      NormalizedCall{Agent: AgentCodex, Kind: KindGeneric, Tool: "whatever"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true},
			wantAllow: true,
		},
		{
			name:      "builtin agent tools does NOT allow an mcp call",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "read_file", ServerName: "files"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true},
			wantAllow: false,
		},
		// The coarse toggle covers only the known kinds, so a device that omits or
		// garbles Kind cannot obtain a blanket allow.
		{
			name:      "builtin agent tools does NOT allow an unrecognized kind",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: "definitely-not-a-kind", Tool: "whatever"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true},
			wantAllow: false,
		},
		{
			name:      "builtin agent tools does NOT allow an empty kind",
			call:      NormalizedCall{Agent: AgentClaudeCode, Tool: "whatever"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true},
			wantAllow: false,
		},

		// Coarse: Obot-hosted MCP.
		{
			name:      "obot-hosted toggle allows obot-hosted mcp call",
			call:      NormalizedCall{Agent: AgentCursor, Kind: KindMCP, Tool: "search", ObotHosted: true},
			allowlist: types.EnforcementAllowlist{AllowAllObotHostedMCP: true},
			wantAllow: true,
		},
		{
			name:      "obot-hosted toggle does not allow non-obot-hosted mcp call",
			call:      NormalizedCall{Agent: AgentCursor, Kind: KindMCP, Tool: "search", ObotHosted: false},
			allowlist: types.EnforcementAllowlist{AllowAllObotHostedMCP: true},
			wantAllow: false,
		},

		// Coarse: built-in agent MCP.
		{
			name:      "builtin agent mcp allows a member of the set",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "screenshot", ServerName: "computer-use"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: true,
		},
		{
			// Two of Claude Code's five built-in names contain spaces and capitals, and
			// no tool namespace preserves either, so the comparison falls back to the
			// normalized name.
			name:      "builtin agent mcp allows a member whose name no namespace preserves",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "x", ServerName: "Claude_Preview"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: true,
		},
		{
			name:      "builtin agent mcp denies a non-member server",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "x", ServerName: "some-user-server"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: false,
		},
		{
			// Codex has no built-in MCP servers: its computer-use is an ordinary
			// configured entry installed by the ChatGPT app.
			name:      "builtin agent mcp denies member name under a different agent",
			call:      NormalizedCall{Agent: AgentCodex, Kind: KindMCP, Tool: "x", ServerName: "computer-use"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: false,
		},

		// Specific: URL matching.
		{
			name: "url match scheme+host+default-port",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://mcp.example.com/api"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://mcp.example.com:443/api"}},
			},
			wantAllow: true,
		},
		{
			name: "url mismatch on port",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://mcp.example.com:8443/api"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://mcp.example.com/api"}},
			},
			wantAllow: false,
		},
		{
			name: "url path prefix matches at boundary",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://h.example.com/team/a/mcp"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://h.example.com/team"}},
			},
			wantAllow: true,
		},
		{
			name: "url path prefix does not match mid-segment",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://h.example.com/teamwork"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://h.example.com/team"}},
			},
			wantAllow: false,
		},
		{
			name: "url with no path constraint matches any path",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://h.example.com/anything/here"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://h.example.com"}},
			},
			wantAllow: true,
		},
		{
			name: "url mismatch on host",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://other.example.com/api"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://mcp.example.com/api"}},
			},
			wantAllow: false,
		},

		// Specific: package matching.
		{
			name: "npm package match any version",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Package: npmPkg("@scope/server", "2.3.4")}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@scope/server"}}},
			},
			wantAllow: true,
		},
		{
			name: "npm package exact version match",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Package: npmPkg("@scope/server", "2.3.4")}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@scope/server", Version: "2.3.4"}}},
			},
			wantAllow: true,
		},
		{
			name: "npm package exact version mismatch",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Package: npmPkg("@scope/server", "2.3.4")}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@scope/server", Version: "9.9.9"}}},
			},
			wantAllow: false,
		},
		{
			name: "pypi package match",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Package: pypiPkg("mcp-server-git", "")}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "mcp-server-git"}}},
			},
			wantAllow: true,
		},
		{
			name: "package source mismatch npm vs pypi",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Package: npmPkg("mcp-server-git", "")}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourcePyPI, Name: "mcp-server-git"}}},
			},
			wantAllow: false,
		},

		// Specific: connector matching.
		{
			name: "connector match by display name",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "search", Server: ServerIdentity{Connector: "claude.ai Linear"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "claude.ai Linear"}},
			},
			wantAllow: true,
		},
		{
			name: "connector match is case-insensitive",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "search", Server: ServerIdentity{Connector: "claude.ai Linear"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "CLAUDE.AI LINEAR"}},
			},
			wantAllow: true,
		},
		{
			name: "connector mismatch",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "search", Server: ServerIdentity{Connector: "claude.ai Notion"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "claude.ai Linear"}},
			},
			wantAllow: false,
		},
		{
			name: "connector entry does not match a call that resolved no connector",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "search", Server: ServerIdentity{URL: "https://mcp.linear.app/sse"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "claude.ai Linear"}},
			},
			wantAllow: false,
		},
		{
			name: "connector entry scoped to specific tools",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "delete_issue", Server: ServerIdentity{Connector: "claude.ai Linear"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "claude.ai Linear", Tools: []string{"search_issues"}}},
			},
			wantAllow: false,
		},
		{
			name: "shell call is NOT allowed by a matching connector entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindShell, Tool: "bash", Server: ServerIdentity{Connector: "claude.ai Linear"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Connector: "claude.ai Linear"}},
			},
			wantAllow: false,
		},

		// Specific: hostname matching.
		{
			name: "hostname match against explicit hostname",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Hostname: "gitmcp.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
			},
			wantAllow: true,
		},
		{
			name: "hostname match derived from url",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://gitmcp.io/owner/repo"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "GitMCP.io"}},
			},
			wantAllow: true,
		},
		{
			name: "hostname mismatch",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{Hostname: "evil.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
			},
			wantAllow: false,
		},

		// Tool-in-server matching.
		{
			name: "empty tools list allows any tool on matched server",
			call: NormalizedCall{Kind: KindMCP, Tool: "delete_everything", Server: ServerIdentity{Hostname: "gitmcp.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
			},
			wantAllow: true,
		},
		{
			name: "listed tool allowed on matched server",
			call: NormalizedCall{Kind: KindMCP, Tool: "read_file", Server: ServerIdentity{Hostname: "gitmcp.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io", Tools: []string{"read_file", "list_files"}}},
			},
			wantAllow: true,
		},
		{
			name: "unlisted tool denied on matched server",
			call: NormalizedCall{Kind: KindMCP, Tool: "write_file", Server: ServerIdentity{Hostname: "gitmcp.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io", Tools: []string{"read_file", "list_files"}}},
			},
			wantAllow: false,
		},

		// Server entries scope MCP calls only.
		{
			name: "shell call is NOT allowed by a matching url entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindShell, Tool: "search", Server: ServerIdentity{URL: "https://gitmcp.io/docs"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs", Tools: []string{"search"}}},
			},
			wantAllow: false,
		},
		{
			name: "write call is NOT allowed by a matching hostname entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindWrite, Tool: "anything", Server: ServerIdentity{Hostname: "gitmcp.io"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}},
			},
			wantAllow: false,
		},
		{
			name: "shell call is NOT allowed by a matching package entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindShell, Tool: "rm", Server: ServerIdentity{Package: &PackageIdentity{Source: types.AllowlistServerPackageSourceNPM, Name: "@modelcontextprotocol/server-filesystem"}}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Package: &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "@modelcontextprotocol/server-filesystem"}}},
			},
			wantAllow: false,
		},
		{
			name: "unrecognized kind is NOT allowed by a matching url entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: "mcp_lookalike", Tool: "search", Server: ServerIdentity{URL: "https://gitmcp.io/docs"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}},
			},
			wantAllow: false,
		},
		{
			name: "empty kind is NOT allowed by a matching url entry",
			call: NormalizedCall{Agent: AgentClaudeCode, Tool: "search", Server: ServerIdentity{URL: "https://gitmcp.io/docs"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}},
			},
			wantAllow: false,
		},
		{
			name: "mcp call with the same identity IS allowed",
			call: NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "search", Server: ServerIdentity{URL: "https://gitmcp.io/docs"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs", Tools: []string{"search"}}},
			},
			wantAllow: true,
		},

		// Deny-by-default fallthrough.
		{
			name:      "empty allowlist denies",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://mcp.example.com"}},
			allowlist: types.EnforcementAllowlist{},
			wantAllow: false,
		},
		{
			name:      "empty allowlist denies non-mcp call",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindShell, Tool: "bash"},
			allowlist: types.EnforcementAllowlist{},
			wantAllow: false,
		},
		{
			name: "malformed entry with no dimension matches nothing",
			call: NormalizedCall{Kind: KindMCP, Tool: "t", Server: ServerIdentity{URL: "https://mcp.example.com"}},
			allowlist: types.EnforcementAllowlist{
				Servers: []types.AllowlistServer{{Tools: []string{"t"}}},
			},
			wantAllow: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.call, tt.allowlist)
			if got.Allow != tt.wantAllow {
				t.Fatalf("Evaluate() Allow = %v, want %v (reason: %q)", got.Allow, tt.wantAllow, got.Reason)
			}
			if got.Reason == "" {
				t.Errorf("Evaluate() returned an empty reason")
			}
		})
	}
}

func TestEvaluateUnresolvedIsDeniedUnderEveryAllowlistShape(t *testing.T) {
	call := NormalizedCall{
		Agent:            AgentClaudeCode,
		Tool:             "search",
		Kind:             KindMCP,
		ServerName:       "linear",
		Unresolved:       true,
		UnresolvedReason: `stdio command "node" is not a supported package runner`,
		Server:           ServerIdentity{Command: "node"},
	}

	for name, allowlist := range map[string]types.EnforcementAllowlist{
		"empty":               {},
		"allow everything":    {AllowEverything: true},
		"builtin agent tools": {AllowAllBuiltinAgentTools: true},
		"builtin agent mcp":   {AllowAllBuiltinAgentMCP: true},
		"obot hosted mcp":     {AllowAllObotHostedMCP: true},
		"every toggle at once": {
			AllowEverything:           true,
			AllowAllObotHostedMCP:     true,
			AllowAllBuiltinAgentTools: true,
			AllowAllBuiltinAgentMCP:   true,
		},
		"matching hostname entry":  {Servers: []types.AllowlistServer{{Hostname: "gitmcp.io"}}},
		"matching connector entry": {Servers: []types.AllowlistServer{{Connector: "claude.ai Linear"}}},
	} {
		t.Run(name, func(t *testing.T) {
			got := Evaluate(call, allowlist)
			if got.Allow {
				t.Fatalf("Evaluate() allowed an unresolved call (reason: %q)", got.Reason)
			}
			if got.Reason != call.UnresolvedReason {
				t.Fatalf("Evaluate() Reason = %q, want the device's reason %q", got.Reason, call.UnresolvedReason)
			}
		})
	}

	// A non-MCP kind is never marked by the device as unresolved,
	// but we'll test that situation here just in case.
	shell := NormalizedCall{Agent: AgentCodex, Tool: "bash", Kind: KindShell, Unresolved: true, UnresolvedReason: "why not"}
	if got := Evaluate(shell, types.EnforcementAllowlist{AllowAllBuiltinAgentTools: true}); got.Allow {
		t.Fatalf("Evaluate() allowed an unresolved shell call (reason: %q)", got.Reason)
	}
}

func TestEvaluateUnresolvedWithoutReasonStillExplainsItself(t *testing.T) {
	for name, reason := range map[string]string{
		"absent":          "",
		"whitespace only": "  \t\n ",
	} {
		t.Run(name, func(t *testing.T) {
			got := Evaluate(
				NormalizedCall{Agent: AgentCursor, Tool: "search", Kind: KindMCP, Unresolved: true, UnresolvedReason: reason},
				types.EnforcementAllowlist{AllowEverything: true},
			)
			if got.Allow {
				t.Fatalf("Evaluate() allowed an unresolved call (reason: %q)", got.Reason)
			}
			if got.Reason != defaultUnresolvedReason {
				t.Fatalf("Evaluate() Reason = %q, want the generic fallback %q", got.Reason, defaultUnresolvedReason)
			}
		})
	}
}

// TestEvaluateResolvedCallIgnoresAStaleReason proves the reason string alone
// does not deny: only the flag does.
func TestEvaluateResolvedCallIgnoresAStaleReason(t *testing.T) {
	call := NormalizedCall{
		Agent:            AgentClaudeCode,
		Tool:             "search",
		Kind:             KindMCP,
		UnresolvedReason: "left over from an earlier attempt",
		Server:           ServerIdentity{URL: "https://gitmcp.io/docs"},
	}
	allowlist := types.EnforcementAllowlist{Servers: []types.AllowlistServer{{URL: "https://gitmcp.io/docs"}}}
	if got := Evaluate(call, allowlist); !got.Allow {
		t.Fatalf("Evaluate() denied a resolved call carrying only a reason string (reason: %q)", got.Reason)
	}
}
