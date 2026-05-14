package devicescan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func writeHermesConfig(t *testing.T, home, body string) {
	t.Helper()
	dir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .hermes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

type hermesScanResult struct {
	state *scanState
	mcps  []types.DeviceScanMCPServer
}

func runHermesScan(t *testing.T, home string) hermesScanResult {
	t.Helper()
	s := newScanState(os.DirFS(home), home)
	mcps := hermesScanner{}.ScanGlobal(s)
	return hermesScanResult{state: s, mcps: mcps}
}

func TestScanHermes_Stdio(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
mcp_servers:
  github:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: secret
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 1 {
		t.Fatalf("want 1 server, got %d", len(r.mcps))
	}
	s := r.mcps[0]
	if s.Client != "hermes" || s.Name != "github" || s.Transport != "stdio" {
		t.Fatalf("unexpected server: %+v", s)
	}
	if s.Command != "npx" || len(s.Args) != 2 || s.Args[0] != "-y" {
		t.Fatalf("command/args wrong: %+v", s)
	}
	if len(s.EnvKeys) != 1 || s.EnvKeys[0] != "GITHUB_PERSONAL_ACCESS_TOKEN" {
		t.Fatalf("env keys wrong: %+v", s.EnvKeys)
	}
	if len(s.HeaderKeys) != 0 {
		t.Fatalf("expected no header keys, got %+v", s.HeaderKeys)
	}
	if s.ConfigHash == "" {
		t.Fatalf("ConfigHash empty")
	}
	wantFile := filepath.Join(home, ".hermes", "config.yaml")
	if s.File != wantFile {
		t.Fatalf("File = %q, want %q", s.File, wantFile)
	}
}

func TestScanHermes_HTTP(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
mcp_servers:
  remote:
    url: https://mcp.example.com/mcp
    headers:
      Authorization: "Bearer xxx"
      X-Tenant: acme
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 1 {
		t.Fatalf("want 1 server, got %d", len(r.mcps))
	}
	s := r.mcps[0]
	if s.Transport != "streamable-http" {
		t.Fatalf("Transport = %q, want streamable-http", s.Transport)
	}
	if s.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("URL = %q", s.URL)
	}
	if len(s.HeaderKeys) != 2 || s.HeaderKeys[0] != "Authorization" || s.HeaderKeys[1] != "X-Tenant" {
		t.Fatalf("header keys wrong (expect sorted): %+v", s.HeaderKeys)
	}
	if s.Command != "" || len(s.Args) != 0 {
		t.Fatalf("expected no command/args for HTTP server: %+v", s)
	}
}

func TestScanHermes_EnabledFalseSkips(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
mcp_servers:
  off:
    command: npx
    args: ["-y", "x"]
    enabled: false
  on:
    command: npx
    args: ["-y", "y"]
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 1 {
		t.Fatalf("want 1 server, got %d (%+v)", len(r.mcps), r.mcps)
	}
	if r.mcps[0].Name != "on" {
		t.Fatalf("expected only `on`, got %q", r.mcps[0].Name)
	}
}

func TestScanHermes_EnabledAbsentDefaultsTrue(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
mcp_servers:
  default_on:
    command: npx
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 1 {
		t.Fatalf("want 1 server, got %d", len(r.mcps))
	}
}

func TestScanHermes_NeitherCommandNorURLSkips(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
mcp_servers:
  empty:
    timeout: 30
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 0 {
		t.Fatalf("expected 0 servers, got %d (%+v)", len(r.mcps), r.mcps)
	}
}

func TestScanHermes_MissingMCPServersKey(t *testing.T) {
	home := t.TempDir()
	writeHermesConfig(t, home, `
other_section:
  foo: bar
`)

	r := runHermesScan(t, home)
	if len(r.mcps) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(r.mcps))
	}
	// Config still recorded as a global file even if empty.
	wantFile := filepath.Join(home, ".hermes", "config.yaml")
	if _, ok := r.state.files[wantFile]; !ok {
		t.Fatalf("expected config file recorded, files=%+v", r.state.files)
	}
}

func TestScanHermes_NoConfigFile(t *testing.T) {
	home := t.TempDir()
	r := runHermesScan(t, home)
	if len(r.mcps) != 0 {
		t.Fatalf("expected 0 servers, got %d", len(r.mcps))
	}
	if len(r.state.files) != 0 {
		t.Fatalf("expected no files recorded, got %+v", r.state.files)
	}
}

func TestScanHermes_SkillsAttribution(t *testing.T) {
	home := t.TempDir()
	skillDir := filepath.Join(home, ".hermes", "skills", "official", "apple", "apple-notes")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	skillBody := "---\nname: apple-notes\ndescription: Manage Apple Notes\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	scan, err := Scan(context.Background(), os.DirFS(home), home, 8)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var found bool
	for _, sk := range scan.Skills {
		if sk.Client == "hermes" && sk.Name == "apple-notes" {
			found = true
			if sk.Description != "Manage Apple Notes" {
				t.Fatalf("Description = %q", sk.Description)
			}
		}
	}
	if !found {
		t.Fatalf("hermes skill not found in scan, skills=%+v", scan.Skills)
	}
}
