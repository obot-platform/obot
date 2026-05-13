package devicescan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var zedSettingsPath = filepath.Join(homeDir, ".config/zed/settings.json")

var zed = client{
	name:      "zed",
	binaries:  []string{"zed"},
	appBundle: "Zed.app",
	directRules: []parseRule{
		{target: zedSettingsPath, parse: parseZedSettings},
		// macOS-only extensions tree. On Linux/Windows os.Stat will
		// return ENOENT and parseZedExtensions will skip.
		{target: filepath.Join(homeDir, "Library/Application Support/Zed/extensions/installed"), parse: parseZedExtensions},
	},
	walkRules: []parseRule{
		{target: ".zed/settings.json", parse: parseZedSettings},
	},
}

// zedSettings has only the field we care about — Zed's
// `context_servers` map keys are opaque server names; values follow
// Zed's own schema.
type zedSettings struct {
	ContextServers map[string]zedContextServer `json:"context_servers"`
}

type zedContextServer struct {
	URL     string         `json:"url"`
	Command string         `json:"command"`
	Args    []string       `json:"args"`
	Env     map[string]any `json:"env"`
	Headers map[string]any `json:"headers"`
	Enabled *bool          `json:"enabled"`
}

// parseZedSettings handles both ~/.config/zed/settings.json and any
// project-scope hit. Project rel is two levels deep inside the owning
// project (<proj>/.zed/settings.json). Emits one MCP observation per
// context_servers entry: URL → sse; command → stdio; neither
// (settings-only extension placeholders) → drop.
func parseZedSettings(path string) parseResult {
	cfg, ok := readJSON[zedSettings](path)
	if !ok {
		return parseResult{}
	}

	var projectPath string
	if path != zedSettingsPath {
		projectPath = filepath.Dir(filepath.Dir(path))
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.ContextServers) {
		e := cfg.ContextServers[name]
		if e.Enabled != nil && !*e.Enabled {
			continue
		}

		switch {
		case e.URL != "":
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:      "zed",
				ProjectPath: projectPath,
				File:        path,
				Name:        name,
				Transport:   "sse",
				URL:         e.URL,
				HeaderKeys:  sortedMapKeys(e.Headers),
				ConfigHash:  mcpConfigHash(name, "sse", "", nil, e.URL),
			})
		case e.Command != "":
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:      "zed",
				ProjectPath: projectPath,
				File:        path,
				Name:        name,
				Transport:   "stdio",
				Command:     e.Command,
				Args:        e.Args,
				EnvKeys:     sortedMapKeys(e.Env),
				ConfigHash:  mcpConfigHash(name, "stdio", e.Command, e.Args, ""),
			})
		}
	}

	return out
}

// parseZedExtensions scans the extensions tree for folders prefixed
// mcp-server- and emits one stdio observation per extension that does
// not already appear in the global settings (i.e. the user has the
// extension installed but has not configured a context_servers entry
// for it). Command/args are blank because the extension supplies them
// at runtime.
func parseZedExtensions(dirPath string) parseResult {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return parseResult{}
	}

	var (
		cfg, _   = readJSON[zedSettings](zedSettingsPath)
		existing = map[string]struct{}{}
	)
	for name, e := range cfg.ContextServers {
		if e.URL == "" && e.Command == "" {
			continue
		}

		existing[name] = struct{}{}
	}

	// File on observations is the settings path when it exists; blank
	// otherwise (extension is installed but has no settings to point at).
	var configPath string
	if fileExists(zedSettingsPath) {
		configPath = zedSettingsPath
	}

	var out parseResult
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		if !strings.HasPrefix(e.Name(), "mcp-server-") {
			continue
		}

		name := e.Name()
		if _, ok := existing[name]; ok {
			continue
		}

		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:     "zed",
			File:       configPath,
			Name:       name,
			Transport:  "stdio",
			ConfigHash: mcpConfigHash(name, "stdio", "", nil, ""),
		})
	}
	return out
}
