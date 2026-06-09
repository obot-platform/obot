package devicescan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/obot-platform/obot/apiclient/types"
)

// runScanFS runs Scan against an in-memory fs with a neutralised
// presence environment (no real $PATH, no /Applications). Returns the
// scan manifest for assertions.
func runScanFS(t *testing.T, files map[string]string) types.DeviceScanManifest {
	t.Helper()
	mapfs := fstest.MapFS{}
	for p, body := range files {
		mapfs[p] = &fstest.MapFile{Data: []byte(body)}
	}

	t.Setenv("PATH", t.TempDir())
	t.Setenv("OPENCLAW_PROFILE", "")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	scan, err := Scan(context.Background(), mapfs, "/home/test", 8)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return scan
}

// findServer returns the first MCP server matching client+name, or nil.
func findServer(scan types.DeviceScanManifest, client, name string) *types.DeviceScanMCPServer {
	for i, m := range scan.MCPServers {
		if m.Client == client && m.Name == name {
			return &scan.MCPServers[i]
		}
	}
	return nil
}

// TestScanners_Smoke covers each scanner with one happy-path config
// (stdio or http, whichever is most natural) and asserts the server is
// emitted with the expected client + transport. The orchestrator,
// walker, build(), and per-scanner toServer logic are all exercised.
func TestScanners_Smoke(t *testing.T) {
	cases := []struct {
		name      string
		client    string
		serverNm  string
		transport string
		files     map[string]string
	}{
		{
			name:      "claude_code stdio",
			client:    "claude_code",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".claude.json": `{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`,
			},
		},
		{
			name:      "claude_desktop stdio",
			client:    "claude_desktop",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				"Library/Application Support/Claude/claude_desktop_config.json": `{"mcpServers":{"github":{"command":"npx","args":["-y","x"]}}}`,
			},
		},
		{
			name:      "codex stdio",
			client:    "codex",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".codex/config.toml": "[mcp_servers.github]\ncommand = \"npx\"\nargs = [\"-y\", \"x\"]\n",
			},
		},
		{
			name:      "cursor stdio",
			client:    "cursor",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".cursor/mcp.json": `{"mcpServers":{"github":{"command":"npx","args":["-y","x"]}}}`,
			},
		},
		{
			name:      "goose stdio",
			client:    "goose",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".config/goose/config.yaml": "extensions:\n  github:\n    type: stdio\n    cmd: npx\n    args: [\"-y\", \"x\"]\n    enabled: true\n",
			},
		},
		{
			name:      "hermes http",
			client:    "hermes",
			serverNm:  "remote",
			transport: "streamable-http",
			files: map[string]string{
				".hermes/config.yaml": "mcp_servers:\n  remote:\n    url: https://mcp.example.com/mcp\n",
			},
		},
		{
			name:      "opencode local",
			client:    "opencode",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".config/opencode/opencode.json": `{"mcp":{"github":{"type":"local","command":["npx","-y","x"]}}}`,
			},
		},
		{
			name:      "vscode stdio",
			client:    "vscode",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				"Library/Application Support/Code/User/mcp.json": `{"servers":{"github":{"command":"npx","args":["-y","x"]}}}`,
			},
		},
		{
			name:      "windsurf stdio",
			client:    "windsurf",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".codeium/windsurf/mcp_config.json": `{"mcpServers":{"github":{"command":"npx","args":["-y","x"]}}}`,
			},
		},
		{
			name:      "zed stdio",
			client:    "zed",
			serverNm:  "github",
			transport: "stdio",
			files: map[string]string{
				".config/zed/settings.json": `{"context_servers":{"github":{"command":"npx","args":["-y","x"]}}}`,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scan := runScanFS(t, c.files)
			s := findServer(scan, c.client, c.serverNm)
			if s == nil {
				t.Fatalf("no server emitted for client=%q name=%q; got %+v", c.client, c.serverNm, scan.MCPServers)
			}
			if s.Transport != c.transport {
				t.Errorf("Transport = %q, want %q", s.Transport, c.transport)
			}
			if s.ConfigHash == "" {
				t.Errorf("ConfigHash empty")
			}
			// build() must synthesise a clients[] row whenever an
			// observation references a client, even if presence didn't
			// fire in the test environment.
			var clientFound bool
			for _, cl := range scan.Clients {
				if cl.Name == c.client {
					clientFound = true
					if !cl.HasMCPServers {
						t.Errorf("HasMCPServers = false for client %q", c.client)
					}
				}
			}
			if !clientFound {
				t.Errorf("no clients[] row synthesised for %q", c.client)
			}
		})
	}
}

// TestScan_DisabledServerSkipped covers the per-scanner rule that an
// explicit `enabled = false` removes a server from the output. Codex
// (TOML) is exercised here; hermes_test.go covers the YAML path; goose
// inverts the default (must be explicit true).
func TestScan_DisabledServerSkipped(t *testing.T) {
	scan := runScanFS(t, map[string]string{
		".codex/config.toml": "[mcp_servers.on]\ncommand = \"x\"\n\n[mcp_servers.off]\ncommand = \"y\"\nenabled = false\n",
	})
	if findServer(scan, "codex", "off") != nil {
		t.Errorf("disabled server emitted")
	}
	if findServer(scan, "codex", "on") == nil {
		t.Errorf("enabled server missing")
	}
}

// TestScan_ProjectScopeWalk verifies the walker dispatches a
// project-scope config to its owning scanner with the project root
// resolved correctly.
func TestScan_ProjectScopeWalk(t *testing.T) {
	scan := runScanFS(t, map[string]string{
		"projects/foo/.cursor/mcp.json": `{"mcpServers":{"github":{"command":"npx"}}}`,
	})
	s := findServer(scan, "cursor", "github")
	if s == nil {
		t.Fatalf("no project-scope server emitted; got %+v", scan.MCPServers)
	}
	if s.ProjectPath == "" {
		t.Errorf("ProjectPath empty for project-scope hit; want non-empty")
	}
}

func TestScan_SkillMultiClientAttribution(t *testing.T) {
	skillFiles := map[string]string{
		".agents/skills/shared-skill/SKILL.md": "---\nname: shared-skill\ndescription: shared\n---\n",
		".claude/skills/claude-skill/SKILL.md": "---\nname: claude-skill\ndescription: claude\n---\n",
		".codex/skills/codex-skill/SKILL.md":   "---\nname: codex-skill\ndescription: codex\n---\n",
	}

	t.Run("compatible clients absent", func(t *testing.T) {
		scan := runScanFS(t, skillFiles)

		assertSkillClients(t, scan, "shared-skill", []string{multiClient})
		assertSkillClients(t, scan, "claude-skill", []string{"claude_code"})
		assertSkillClients(t, scan, "codex-skill", []string{"codex"})
		for _, c := range scan.Clients {
			if c.Name == "vscode" || c.Name == "cursor" {
				t.Errorf("%s client synthesized without presence: %+v", c.Name, c)
			}
		}
	})

	t.Run("compatible clients present", func(t *testing.T) {
		mapfs := fstest.MapFS{}
		for p, body := range skillFiles {
			mapfs[p] = &fstest.MapFile{Data: []byte(body)}
		}

		// Presence detection uses real OS access against homeAbs, so
		// give the scan a real home containing .vscode and .cursor
		// config dirs.
		home := t.TempDir()
		for _, dir := range []string{".vscode", ".cursor"} {
			if err := os.Mkdir(filepath.Join(home, dir), 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dir, err)
			}
		}
		t.Setenv("PATH", t.TempDir())
		t.Setenv("OPENCLAW_PROFILE", "")
		clientAppBundleDirs = []string{t.TempDir()}
		t.Cleanup(func() { clientAppBundleDirs = nil })

		scan, err := Scan(context.Background(), mapfs, home, 8)
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}

		assertSkillClients(t, scan, "shared-skill", []string{multiClient, "cursor", "vscode"})
		assertSkillClients(t, scan, "claude-skill", []string{"claude_code", "cursor", "vscode"})
		assertSkillClients(t, scan, "codex-skill", []string{"codex", "cursor"})
	})
}

func TestScan_CursorSkillLocations(t *testing.T) {
	scan := runScanFS(t, map[string]string{
		// Nested category subdir under the home root — Cursor walks
		// skills roots recursively.
		".cursor/skills/frontend/react-pro/SKILL.md": "---\nname: react-pro\ndescription: nested\n---\n",
		// Project-level .cursor/skills anywhere in a repo.
		"work/repo/.cursor/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: project\n---\n",
	})

	assertSkillClients(t, scan, "react-pro", []string{"cursor"})
	assertSkillClients(t, scan, "deploy", []string{"cursor"})
	for _, skill := range scan.Skills {
		switch skill.Name {
		case "react-pro":
			if skill.ProjectPath != "" {
				t.Errorf("react-pro ProjectPath: want empty (user scope), got %q", skill.ProjectPath)
			}
		case "deploy":
			if skill.ProjectPath != "/home/test/work/repo" {
				t.Errorf("deploy ProjectPath: want /home/test/work/repo, got %q", skill.ProjectPath)
			}
		}
	}
}

func TestScan_ProjectSkillDiscoveredAtDefaultDepth(t *testing.T) {
	mapfs := fstest.MapFS{
		"projects/deep/org/repo/.github/skills/shared-skill/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: shared-skill\ndescription: shared\n---\n"),
		},
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OPENCLAW_PROFILE", "")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	scan, err := Scan(context.Background(), mapfs, "/home/test", 5)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	assertSkillClients(t, scan, "shared-skill", []string{multiClient})
	for _, skill := range scan.Skills {
		if skill.Name == "shared-skill" && skill.ProjectPath != "/home/test/projects/deep/org/repo" {
			t.Fatalf("ProjectPath: want /home/test/projects/deep/org/repo, got %q", skill.ProjectPath)
		}
	}
}

func TestScan_SkillProjectScopeInference(t *testing.T) {
	scan := runScanFS(t, map[string]string{
		// A client dot-dir nested inside a project keeps project scope.
		"work/repo/.windsurf/skills/deploy/SKILL.md": "---\nname: deploy-windsurf\ndescription: project\n---\n",
		// A project skill root at the top of the walk resolves to the
		// home directory as the project root, not the skill dir itself.
		".github/skills/home-skill/SKILL.md": "---\nname: home-skill\ndescription: home repo skill\n---\n",
	})

	assertSkillClients(t, scan, "deploy-windsurf", []string{"windsurf"})
	assertSkillClients(t, scan, "home-skill", []string{multiClient})
	for _, skill := range scan.Skills {
		switch skill.Name {
		case "deploy-windsurf":
			if skill.ProjectPath != "/home/test/work/repo" {
				t.Errorf("deploy-windsurf ProjectPath: want /home/test/work/repo, got %q", skill.ProjectPath)
			}
		case "home-skill":
			if skill.ProjectPath != "/home/test" {
				t.Errorf("home-skill ProjectPath: want /home/test, got %q", skill.ProjectPath)
			}
		}
	}
}

func assertSkillClients(t *testing.T, scan types.DeviceScanManifest, name string, want []string) {
	t.Helper()
	clients := map[string]struct{}{}
	for _, skill := range scan.Skills {
		if skill.Name != name {
			continue
		}
		clients[skill.Client] = struct{}{}
		for _, client := range skill.Clients {
			clients[client] = struct{}{}
		}
	}
	if len(clients) != len(want) {
		t.Fatalf("skill %q clients: want %v, got %v (skills=%+v)", name, want, clients, scan.Skills)
	}
	for _, client := range want {
		if _, ok := clients[client]; !ok {
			t.Fatalf("skill %q missing client %q in %v", name, client, clients)
		}
	}
}
