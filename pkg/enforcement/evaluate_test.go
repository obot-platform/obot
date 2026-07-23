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
			call:      NormalizedCall{Agent: AgentCodex, Kind: KindMCP, Tool: "screenshot", ServerName: "computer-use"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: true,
		},
		{
			name:      "builtin agent mcp denies a non-member server",
			call:      NormalizedCall{Agent: AgentCodex, Kind: KindMCP, Tool: "x", ServerName: "some-user-server"},
			allowlist: types.EnforcementAllowlist{AllowAllBuiltinAgentMCP: true},
			wantAllow: false,
		},
		{
			name:      "builtin agent mcp denies member name under a different agent",
			call:      NormalizedCall{Agent: AgentClaudeCode, Kind: KindMCP, Tool: "x", ServerName: "computer-use"},
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
