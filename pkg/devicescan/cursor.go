package devicescan

import (
	"os"
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

var (
	cursorMCPPath      = filepath.Join(homeDir, ".cursor/mcp.json")
	cursorSettingsPath = filepath.Join(homeDir, ".cursor/settings.json")
)

var cursor = client{
	name:      "cursor",
	binaries:  []string{"cursor"},
	appBundle: "Cursor.app",
	directRules: []parseRule{
		{target: cursorMCPPath, parse: parseCursorMCP},
		{target: filepath.Join(homeDir, ".cursor/plugins/cache/cursor-public"), parse: parseCursorPlugins},
	},
	walkRules: []parseRule{
		{target: ".cursor/mcp.json", parse: parseCursorMCP},
	},
	// The direct path above owns this subtree; the central walk
	// shouldn't descend into it (avoids double-emitting nested SKILL.md
	// hits via the multi-client projectSkills rule).
	walkSkipPrefixes: []string{".cursor/plugins"},
}

// parseCursorMCP handles both the global .cursor/mcp.json and any
// project-scope hit. Global is recognized by exact-path equality;
// otherwise the parent two directories up is the project root.
func parseCursorMCP(path string) parseResult {
	cfg, ok := readJSON[struct {
		MCPServers map[string]mcpServerSpec `json:"mcpServers"`
	}](path)
	if !ok {
		return parseResult{}
	}

	var projectPath string
	if path != cursorMCPPath {
		projectPath = filepath.Dir(filepath.Dir(path))
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e         = cfg.MCPServers[name]
			transport = normalizeTransport(e.Type, e.Transport, e.URL)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:      "cursor",
			ProjectPath: projectPath,
			File:        path,
			Name:        name,
			Transport:   transport,
			Command:     e.Command,
			Args:        e.Args,
			URL:         e.URL,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  sortedMapKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
		})
	}

	return out
}

// parseCursorPlugins walks
// .cursor/plugins/cache/cursor-public/<name>/<hash>/ and emits one
// plugin observation per plugin. Dedupes by plugin name (first hash
// dir wins). Resolves enabled state from .cursor/settings.json,
// checking both "<name>@cursor-public" and "<name>" key forms.
func parseCursorPlugins(dirPath string) parseResult {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return parseResult{}
	}

	var (
		settings, _ = readJSON[struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}](cursorSettingsPath)
		seen = map[string]struct{}{}
		out  parseResult
	)

	for _, p := range entries {
		if _, ok := seen[p.Name()]; !p.IsDir() || ok {
			continue
		}

		pluginPath := filepath.Join(dirPath, p.Name())
		hashes, err := os.ReadDir(pluginPath)
		if err != nil {
			log.Debugf("cursor: skipping plugin %q: %v", pluginPath, err)
			continue
		}
		for _, h := range hashes {
			if !h.IsDir() {
				continue
			}

			var (
				installPath  = filepath.Join(pluginPath, h.Name())
				manifestPath = filepath.Join(installPath, ".cursor-plugin/plugin.json")
			)
			if !fileExists(manifestPath) {
				continue
			}

			// enabled state: settings.json may key this plugin with either
			// the marketplace-qualified or bare-name form; first hit wins,
			// and absence in both means the plugin is on disk but disabled.
			var enabled bool
			for _, key := range []string{p.Name() + "@cursor-public", p.Name()} {
				if v, ok := settings.EnabledPlugins[key]; ok {
					enabled = v
					break
				}
			}

			sub := parsePluginInstall(installPath, manifestPath, pluginInstallOpts{
				client:       "cursor",
				pluginType:   "cursor_plugin",
				marketplace:  "cursor-public",
				enabled:      enabled,
				nameFallback: p.Name(),
				nestedMCPRel: []string{"mcp.json", ".mcp.json"},
			})
			out.merge(sub)

			seen[p.Name()] = struct{}{}
			break
		}
	}

	return out
}
