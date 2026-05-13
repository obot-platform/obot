package devicescan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestHighestVersionDirUsesSemver(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1.9.0", "1.10.0", "2.0.0-rc.1", "2.0.0", "not-a-version"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "3.0.0"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	versionPath, version, ok := highestVersionDir(dir)
	if !ok {
		t.Fatal("expected a version directory")
	}
	if version != "2.0.0" {
		t.Fatalf("version = %q, want %q", version, "2.0.0")
	}
	if versionPath != filepath.Join(dir, "2.0.0") {
		t.Fatalf("versionPath = %q, want %q", versionPath, filepath.Join(dir, "2.0.0"))
	}
}

func TestParseCodexConfig(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	path := filepath.Join(projectDir, ".codex", "config.toml")
	writeTestFile(t, path, `
[mcp_servers.zeta]
command = "npx"
args = ["server-z"]

[mcp_servers.alpha]
url = "https://example.com/mcp"
bearer_token_env_var = "BEARER_TOKEN"

[mcp_servers.alpha.env]
FOO = "bar"

[mcp_servers.alpha.http_headers]
"X-Static" = "secret"

[mcp_servers.alpha.env_http_headers]
"X-Env" = "ENV_TOKEN"
`)

	got := parseCodexConfig(path)
	if len(got.files) != 1 || got.files[0].Path != path {
		t.Fatalf("files = %#v, want one entry for %q", got.files, path)
	}
	if len(got.mcps) != 2 {
		t.Fatalf("mcps = %#v, want two entries", got.mcps)
	}

	alpha := got.mcps[0]
	if alpha.Name != "alpha" {
		t.Fatalf("first MCP name = %q, want alpha", alpha.Name)
	}
	if alpha.ProjectPath != projectDir {
		t.Fatalf("projectPath = %q, want %q", alpha.ProjectPath, projectDir)
	}
	if alpha.Transport != "streamable-http" {
		t.Fatalf("transport = %q, want streamable-http", alpha.Transport)
	}
	if !slices.Equal(alpha.EnvKeys, []string{"FOO"}) {
		t.Fatalf("envKeys = %#v, want [FOO]", alpha.EnvKeys)
	}
	if !slices.Equal(alpha.HeaderKeys, []string{"Authorization", "X-Env", "X-Static"}) {
		t.Fatalf("headerKeys = %#v, want Authorization/X-Env/X-Static", alpha.HeaderKeys)
	}

	if got.mcps[1].Name != "zeta" {
		t.Fatalf("second MCP name = %q, want zeta", got.mcps[1].Name)
	}
}

func TestParseCursorMCP(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "project")
	path := filepath.Join(projectDir, ".cursor", "mcp.json")
	writeTestFile(t, path, `{
  "mcpServers": {
    "zeta": {"command": "npx", "args": ["server-z"]},
    "alpha": {"url": "https://example.com/sse"}
  }
}`)

	got := parseCursorMCP(path)
	if len(got.files) != 1 || got.files[0].Path != path {
		t.Fatalf("files = %#v, want one entry for %q", got.files, path)
	}
	if len(got.mcps) != 2 {
		t.Fatalf("mcps = %#v, want two entries", got.mcps)
	}

	alpha := got.mcps[0]
	if alpha.Name != "alpha" || alpha.Transport != "sse" {
		t.Fatalf("first MCP = %#v, want alpha sse", alpha)
	}
	if alpha.ProjectPath != projectDir {
		t.Fatalf("projectPath = %q, want %q", alpha.ProjectPath, projectDir)
	}

	zeta := got.mcps[1]
	if zeta.Name != "zeta" || zeta.Transport != "stdio" || zeta.Command != "npx" {
		t.Fatalf("second MCP = %#v, want zeta stdio npx", zeta)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
