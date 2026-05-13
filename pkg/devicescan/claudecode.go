package devicescan

import (
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var claudeSettingsPath = filepath.Join(homeDir, ".claude/settings.json")

var claudeCode = client{
	name:     "claude_code",
	binaries: []string{"claude"},
	directRules: []parseRule{
		{target: filepath.Join(homeDir, ".claude.json"), parse: parseClaudeGlobalConfig},
		{target: filepath.Join(homeDir, ".claude/skills"), parse: parseSkillDir("claude_code")},
		{target: filepath.Join(homeDir, ".claude/plugins/installed_plugins.json"), parse: parseClaudePlugins},
	},
	walkRules: []parseRule{
		{target: ".mcp.json", parse: parseClaudeProjectMCP},
	},
	// Direct paths above own these subtrees; the central walk should
	// not descend into them.
	walkSkipPrefixes: []string{".claude/plugins", ".claude/skills"},
}

// claudeGlobalConfig is the shape of ~/.claude.json: a top-level
// mcpServers map plus a projects map keyed by absolute project path,
// each with its own mcpServers block.
type claudeGlobalConfig struct {
	MCPServers map[string]mcpServerSpec `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]mcpServerSpec `json:"mcpServers"`
	} `json:"projects"`
}

// claudeProjectMCPFile is the shape of a project-scope .mcp.json.
type claudeProjectMCPFile struct {
	MCPServers map[string]mcpServerSpec `json:"mcpServers"`
}

// claudePluginsRegistry is the shape of installed_plugins.json: a
// plugins map keyed by "name@marketplace" → list of installations.
type claudePluginsRegistry struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
	} `json:"plugins"`
}

type claudeSettings struct {
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

func parseClaudeGlobalConfig(path string) parseResult {
	cfg, ok := readJSON[claudeGlobalConfig](path)
	if !ok {
		return parseResult{}
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e         = cfg.MCPServers[name]
			transport = normalizeTransport(e.Type, e.Transport, e.URL)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:     "claude_code",
			File:       path,
			Name:       name,
			Transport:  transport,
			Command:    e.Command,
			Args:       e.Args,
			URL:        e.URL,
			EnvKeys:    sortedMapKeys(e.Env),
			HeaderKeys: sortedMapKeys(e.Headers),
			ConfigHash: mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
		})
	}

	for _, projKey := range sortedMapKeys(cfg.Projects) {
		for _, name := range sortedMapKeys(cfg.Projects[projKey].MCPServers) {
			var (
				e         = cfg.Projects[projKey].MCPServers[name]
				transport = normalizeTransport(e.Type, e.Transport, e.URL)
			)
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:      "claude_code",
				ProjectPath: projKey,
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
	}
	return out
}

func parseClaudeProjectMCP(path string) parseResult {
	cfg, ok := readJSON[claudeProjectMCPFile](path)
	if !ok {
		return parseResult{}
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e         = cfg.MCPServers[name]
			transport = normalizeTransport(e.Type, e.Transport, e.URL)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:      "claude_code",
			ProjectPath: filepath.Dir(path),
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

// parseClaudePlugins reads installed_plugins.json and emits one
// plugin observation per installation that resolves to a directory
// on the filesystem, along with nested MCP servers and skills.
func parseClaudePlugins(path string) parseResult {
	registry, ok := readJSON[claudePluginsRegistry](path)
	if !ok || len(registry.Plugins) == 0 {
		return parseResult{}
	}

	var (
		settings, _  = readJSON[claudeSettings](claudeSettingsPath)
		registryFile = readScanFile(path)
	)

	out := parseResult{files: []types.DeviceScanFile{registryFile}}
	for _, pluginKey := range sortedMapKeys(registry.Plugins) {
		var (
			installs                   = registry.Plugins[pluginKey]
			pluginName, marketplace, _ = strings.Cut(pluginKey, "@")
		)

		for _, install := range installs {
			if install.InstallPath == "" {
				continue
			}

			if !dirExists(install.InstallPath) {
				continue
			}

			manifestPath := filepath.Join(install.InstallPath, ".claude-plugin/plugin.json")
			if !fileExists(manifestPath) {
				continue
			}

			sub := parsePluginInstall(install.InstallPath, manifestPath, pluginInstallOpts{
				client:          "claude_code",
				pluginType:      "claude_code_plugin",
				marketplace:     marketplace,
				enabled:         settings.EnabledPlugins[pluginKey],
				nameFallback:    pluginName,
				versionFallback: install.Version,
				nestedMCPRel:    []string{"mcp.json", ".mcp.json"},
				mcpServerXform:  substituteClaudePluginRoot(install.InstallPath),
			})
			out.merge(sub)
		}
	}

	return out
}

// substituteClaudePluginRoot returns an mcpServerXform that replaces
// ${CLAUDE_PLUGIN_ROOT} with installPath inside the command, args,
// env, and url fields of a parsed mcpServerSpec.
func substituteClaudePluginRoot(installPath string) func(*mcpServerSpec) {
	return func(e *mcpServerSpec) {
		sub := func(s string) string {
			return strings.ReplaceAll(s, "${CLAUDE_PLUGIN_ROOT}", installPath)
		}
		e.Command = sub(e.Command)
		e.URL = sub(e.URL)

		for i, a := range e.Args {
			e.Args[i] = sub(a)
		}

		for k, v := range e.Env {
			if str, ok := v.(string); ok {
				e.Env[k] = sub(str)
			}
		}
	}
}
