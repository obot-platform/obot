package devicescan

import (
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	claudeGlobalConfigRel     = ".claude.json"
	claudeSettingsRel         = ".claude/settings.json"
	claudeInstalledPluginsRel = ".claude/plugins/installed_plugins.json"
	claudePluginManifestSub   = ".claude-plugin/plugin.json"
)

// claudeCodeConfig is the shape of ~/.claude.json: a global mcpServers
// map plus a projects map keyed by absolute project path, each with its
// own mcpServers block.
type claudeCodeConfig struct {
	MCPServers map[string]mcpServerSpec `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]mcpServerSpec `json:"mcpServers"`
	} `json:"projects"`
}

// claudePluginsRegistry is the shape of installed_plugins.json: a
// `plugins` map keyed by "name@marketplace" → list of installations.
type claudePluginsRegistry struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
	} `json:"plugins"`
}

// claudeSettings — only the field we read.
type claudeSettings struct {
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

type claudeCodeScanner struct{}

func (claudeCodeScanner) Name() string { return "claude_code" }

func (claudeCodeScanner) Presence() clientPresenceDef {
	return clientPresenceDef{binaries: []string{"claude"}, configPaths: []string{".claude"}}
}

func (claudeCodeScanner) GlobalConfigPaths() []string { return []string{claudeGlobalConfigRel} }

func (claudeCodeScanner) ProjectGlobs() []string { return []string{"**/.mcp.json"} }

func (claudeCodeScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[claudeCodeConfig](s.fsys, claudeGlobalConfigRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(claudeGlobalConfigRel)

	out := make([]types.DeviceScanMCPServer, 0, len(cfg.MCPServers))
	for name, e := range cfg.MCPServers {
		out = append(out, e.toServer(name, "claude_code", configAbs, ""))
	}
	for projKey, proj := range cfg.Projects {
		for name, e := range proj.MCPServers {
			out = append(out, e.toServer(name, "claude_code", configAbs, projKey))
		}
	}
	return out
}

func (claudeCodeScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	projectAbs := s.abs(path.Dir(configRel))
	return emitJSONServersProject(s, configRel, "mcpServers", "claude_code", projectAbs)
}

// ScanPlugins reads installed_plugins.json and emits a Plugin observation
// (plus nested MCPServer / Skill observations) for each installation that
// resolves to a directory under the home fs.
func (claudeCodeScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	registry, ok := readJSON[claudePluginsRegistry](s.fsys, claudeInstalledPluginsRel)
	if !ok || len(registry.Plugins) == 0 {
		return nil, nil, nil
	}

	settings, _ := readJSON[claudeSettings](s.fsys, claudeSettingsRel)

	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for pluginKey, installs := range registry.Plugins {
		pluginName, marketplace := splitPluginKey(pluginKey)
		for _, install := range installs {
			if install.InstallPath == "" {
				continue
			}
			installRel, ok := relUnderHome(s.homeAbs, install.InstallPath)
			if !ok || !dirExists(s.fsys, installRel) {
				continue
			}
			manifestRel := path.Join(installRel, claudePluginManifestSub)
			if !fileExists(s.fsys, manifestRel) {
				continue
			}
			ep := emitPlugin(s, emitPluginOpts{
				installRel:      installRel,
				manifestRel:     manifestRel,
				pluginType:      "claude_code_plugin",
				client:          "claude_code",
				marketplace:     marketplace,
				enabled:         settings.EnabledPlugins[pluginKey],
				nameFallback:    pluginName,
				versionFallback: install.Version,
				nestedMCPRel:    []string{"mcp.json", ".mcp.json"},
				mcpServerXform:  substituteClaudePluginRoot(install.InstallPath),
			})
			ps = append(ps, ep.plugin)
			ms = append(ms, ep.servers...)
			sks = append(sks, ep.skills...)
		}
	}
	return ps, ms, sks
}

// substituteClaudePluginRoot returns an mcpServerXform that replaces
// ${CLAUDE_PLUGIN_ROOT} with installPathAbs in the command, args, env,
// and url fields of a parsed mcpServerSpec.
func substituteClaudePluginRoot(installPathAbs string) func(*mcpServerSpec) {
	return func(e *mcpServerSpec) {
		sub := func(s string) string {
			return strings.ReplaceAll(s, "${CLAUDE_PLUGIN_ROOT}", installPathAbs)
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

// readEnabledPluginsMap reads enabledPlugins from a settings file (used
// by Cursor as well as Claude Code) and returns it as map[key]bool.
func readEnabledPluginsMap(fsys fs.FS, rel string) map[string]bool {
	type settings struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	out, ok := readJSON[settings](fsys, rel)
	if !ok {
		return nil
	}
	return out.EnabledPlugins
}

// splitPluginKey separates "name@marketplace" plugin keys into their parts.
func splitPluginKey(key string) (name, marketplace string) {
	at := strings.IndexByte(key, '@')
	if at < 0 {
		return key, ""
	}
	return key[:at], key[at+1:]
}

// relUnderHome converts an absolute path into its fs-relative form when
// the path lies under homeAbs. ok=false otherwise.
func relUnderHome(homeAbs, abs string) (string, bool) {
	rel, err := filepath.Rel(homeAbs, abs)
	if err != nil {
		return "", false
	}
	if strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
