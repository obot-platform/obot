package devicescan

import (
	"io/fs"
	"path"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	claudeDesktopAppSupportDir = "Library/Application Support/Claude"
	claudeDesktopExtRel        = claudeDesktopAppSupportDir + "/extensions-installations.json"
	claudeDesktopConfigRel     = claudeDesktopAppSupportDir + "/claude_desktop_config.json"
)

// claudeDesktopExtensions wraps the top-level `extensions` map in
// extensions-installations.json. Each extension carries a manifest with
// nested server info.
type claudeDesktopExtensions struct {
	Extensions map[string]struct {
		Manifest struct {
			DisplayName string `json:"display_name"`
			Server      struct {
				MCPConfig  *mcpServerSpec `json:"mcp_config"`
				EntryPoint string         `json:"entry_point"`
			} `json:"server"`
		} `json:"manifest"`
	} `json:"extensions"`
}

// claudeDesktopConfig is the JSON shape of claude_desktop_config.json.
// We use it both for MCP server scanning (mcpServers map) and for
// connector plugin enumeration during ScanPlugins.
type claudeDesktopConfig struct {
	MCPServers map[string]mcpServerSpec `json:"mcpServers"`
}

type claudeDesktopScanner struct{}

func (claudeDesktopScanner) Name() string { return "claude_desktop" }

func (claudeDesktopScanner) Presence() clientPresenceDef {
	return clientPresenceDef{
		appBundles:  []string{"Claude.app"},
		configPaths: []string{claudeDesktopAppSupportDir},
	}
}

func (claudeDesktopScanner) GlobalConfigPaths() []string {
	return []string{claudeDesktopExtRel, claudeDesktopConfigRel}
}

func (claudeDesktopScanner) ProjectGlobs() []string { return nil }

func (c claudeDesktopScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	out := scanClaudeDesktopExtensions(s)
	out = append(out, emitJSONServersGlobal(s, claudeDesktopConfigRel, "mcpServers", "claude_desktop")...)
	return out
}

func (claudeDesktopScanner) ScanProject(*scanState, string) []types.DeviceScanMCPServer { return nil }

// ScanPlugins emits three flavors of observation, all tagged
// client=claude_desktop:
//
//  1. One DeviceScanPlugin per Cowork RPM-installed plugin under
//     local-agent-mode-sessions/<X>/<Y>/rpm/plugin_<id>/, plus its
//     nested MCP servers (from .mcp.json) and skills.
//  2. One DeviceScanPlugin per entry in the mcpServers block of
//     claude_desktop_config.json (plugin_type =
//     claude_desktop_connector). The MCP server itself is produced by
//     ScanGlobal; the plugin entry captures the connector as a
//     first-class artifact alongside.
//  3. DeviceScanSkills for every skills/<name>/SKILL.md under
//     local-agent-mode-sessions/skills-plugin/<install-uuid>/<plugin-uuid>/.
//     Cowork ships these as a bundle (the "anthropic-skills" delivery
//     vehicle) — there's no .mcp.json, no commands, no hooks; treating
//     them as a Plugin would inflate the inventory with an empty
//     wrapper. We surface the skills only.
func (c claudeDesktopScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	plugins, servers, skills := c.scanRpmPlugins(s)
	plugins = append(c.scanConnectors(s), plugins...)
	skills = append(c.scanSkillsPlugin(s), skills...)
	return plugins, servers, skills
}

func (claudeDesktopScanner) scanConnectors(s *scanState) []types.DeviceScanPlugin {
	cfg, ok := readJSON[claudeDesktopConfig](s.fsys, claudeDesktopConfigRel)
	if !ok || len(cfg.MCPServers) == 0 {
		return nil
	}
	configAbs := s.addFileOrAbs(claudeDesktopConfigRel)

	out := make([]types.DeviceScanPlugin, 0, len(cfg.MCPServers))
	for name := range cfg.MCPServers {
		out = append(out, types.DeviceScanPlugin{
			Client:        "claude_desktop",
			ConfigPath:    configAbs,
			Name:          name,
			PluginType:    "claude_desktop_connector",
			Enabled:       true,
			Files:         []string{configAbs},
			HasMCPServers: true,
		})
	}
	return out
}

// claudeDesktopSkillsManifest is the shape of <snapshot>/manifest.json.
// We only need lastUpdated for active-snapshot selection; per-skill
// metadata (description, enabled) is already available through SKILL.md
// frontmatter, which ingestSkill reads.
type claudeDesktopSkillsManifest struct {
	LastUpdated int64 `json:"lastUpdated"`
}

// scanSkillsPlugin enumerates Cowork's persistent skills bundle.
// Cowork lays it out as
// <root>/<install-uuid>/<plugin-uuid>/{.claude-plugin/plugin.json,
// manifest.json, skills/<name>/SKILL.md}. Multiple <install-uuid>
// snapshots of the same <plugin-uuid> can coexist on disk; we keep
// only the snapshot with the newest manifest.json:lastUpdated so we
// don't emit phantom skill rows for stale copies Cowork has left
// behind.
//
// We emit ONLY skill rows here. The wrapping .claude-plugin/plugin.json
// is a delivery-vehicle manifest (no MCP servers, no commands, no
// hooks) — treating it as a DeviceScanPlugin would inflate inventory
// with an empty wrapper. Real Cowork plugins live under rpm/ and are
// handled by scanRpmPlugins.
func (claudeDesktopScanner) scanSkillsPlugin(s *scanState) []types.DeviceScanSkill {
	bundleRoot := path.Join(claudeDesktopAppSupportDir, "local-agent-mode-sessions/skills-plugin")
	snapshotInstalls, err := fs.ReadDir(s.fsys, bundleRoot)
	if err != nil {
		return nil
	}

	type snapshot struct {
		installRel  string
		lastUpdated int64
	}
	newestByPluginID := map[string]snapshot{}
	for _, snapshotInstall := range snapshotInstalls {
		if !snapshotInstall.IsDir() {
			continue
		}
		pluginDirs, err := fs.ReadDir(s.fsys, path.Join(bundleRoot, snapshotInstall.Name()))
		if err != nil {
			continue
		}
		for _, pluginDir := range pluginDirs {
			if !pluginDir.IsDir() {
				continue
			}
			installRel := path.Join(bundleRoot, snapshotInstall.Name(), pluginDir.Name())
			if !fileExists(s.fsys, path.Join(installRel, ".claude-plugin/plugin.json")) {
				continue
			}
			var lastUpdated int64
			if manifest, ok := readJSON[claudeDesktopSkillsManifest](s.fsys, path.Join(installRel, "manifest.json")); ok {
				lastUpdated = manifest.LastUpdated
			}
			pluginID := pluginDir.Name()
			existing, seen := newestByPluginID[pluginID]
			if !seen || lastUpdated > existing.lastUpdated || (lastUpdated == existing.lastUpdated && installRel < existing.installRel) {
				newestByPluginID[pluginID] = snapshot{installRel: installRel, lastUpdated: lastUpdated}
			}
		}
	}

	var sks []types.DeviceScanSkill
	for _, newest := range newestByPluginID {
		sks = append(sks, emitNestedSkills(s, newest.installRel, "claude_desktop")...)
	}
	return sks
}

// claudeDesktopRpmManifest is the partial shape of rpm/manifest.json
// we consume. We only read what's not in .claude-plugin/plugin.json:
// the per-plugin marketplaceName. Everything else (name, version,
// description, author) comes from plugin.json via emitPlugin.
type claudeDesktopRpmManifest struct {
	Plugins []struct {
		ID              string `json:"id"`
		MarketplaceName string `json:"marketplaceName"`
	} `json:"plugins"`
}

// scanRpmPlugins enumerates Cowork's server-pushed RPM plugin
// installations. Layout:
//
//	<root>/local-agent-mode-sessions/<X>/<Y>/rpm/
//	    manifest.json                                ← marketplace metadata join
//	    plugin_<id>/                                  ← install root
//	        .claude-plugin/plugin.json                ← name, version, description, author
//	        .mcp.json                                 ← nested MCP servers (optional)
//	        skills/<name>/SKILL.md                    ← nested skills (optional)
//	        README.md
//
// .claude-plugin/plugin.json is the source of truth for plugin
// metadata; rpm/manifest.json supplies marketplaceName (joined by
// plugin id) as enrichment. Install paths are naturally unique across
// rpm/ trees (each is rooted at a distinct <X>/<Y> session context),
// so no dedup is needed.
func (claudeDesktopScanner) scanRpmPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	sessionsRoot := path.Join(claudeDesktopAppSupportDir, "local-agent-mode-sessions")
	outerSessionDirs, err := fs.ReadDir(s.fsys, sessionsRoot)
	if err != nil {
		return nil, nil, nil
	}

	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for _, outerSession := range outerSessionDirs {
		if !outerSession.IsDir() || outerSession.Name() == "skills-plugin" {
			continue
		}
		innerSessionDirs, err := fs.ReadDir(s.fsys, path.Join(sessionsRoot, outerSession.Name()))
		if err != nil {
			continue
		}
		for _, innerSession := range innerSessionDirs {
			if !innerSession.IsDir() {
				continue
			}
			rpmRoot := path.Join(sessionsRoot, outerSession.Name(), innerSession.Name(), "rpm")
			pluginDirs, err := fs.ReadDir(s.fsys, rpmRoot)
			if err != nil {
				continue
			}
			marketplaceByPluginID := map[string]string{}
			if manifest, ok := readJSON[claudeDesktopRpmManifest](s.fsys, path.Join(rpmRoot, "manifest.json")); ok {
				for _, p := range manifest.Plugins {
					if p.ID != "" && p.MarketplaceName != "" {
						marketplaceByPluginID[p.ID] = p.MarketplaceName
					}
				}
			}
			for _, pluginDir := range pluginDirs {
				if !pluginDir.IsDir() || !strings.HasPrefix(pluginDir.Name(), "plugin_") {
					continue
				}
				installRel := path.Join(rpmRoot, pluginDir.Name())
				if !fileExists(s.fsys, path.Join(installRel, ".claude-plugin/plugin.json")) {
					// Half-written snapshot or non-install dir. Skip.
					continue
				}
				ep := emitPlugin(s, emitPluginOpts{
					installRel:   installRel,
					manifestRel:  path.Join(installRel, ".claude-plugin/plugin.json"),
					pluginType:   "claude_desktop_plugin",
					client:       "claude_desktop",
					marketplace:  marketplaceByPluginID[pluginDir.Name()],
					enabled:      true,
					nameFallback: pluginDir.Name(), // "plugin_<id>" when plugin.json lacks name
					nestedMCPRel: []string{".mcp.json"},
				})
				ps = append(ps, ep.plugin)
				ms = append(ms, ep.servers...)
				sks = append(sks, ep.skills...)
			}
		}
	}
	return ps, ms, sks
}

func scanClaudeDesktopExtensions(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[claudeDesktopExtensions](s.fsys, claudeDesktopExtRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(claudeDesktopExtRel)

	out := make([]types.DeviceScanMCPServer, 0, len(cfg.Extensions))
	for name, ext := range cfg.Extensions {
		displayName := name
		if ext.Manifest.DisplayName != "" {
			displayName = ext.Manifest.DisplayName
		}

		var (
			command string
			args    []string
			env     map[string]any
		)
		if mc := ext.Manifest.Server.MCPConfig; mc != nil {
			command = mc.Command
			args = mc.Args
			env = mc.Env
		} else if ep := ext.Manifest.Server.EntryPoint; ep != "" {
			parts := strings.Fields(ep)
			if len(parts) > 0 {
				command = parts[0]
				args = parts[1:]
			}
		}

		out = append(out, types.DeviceScanMCPServer{
			Client:     "claude_desktop",
			File:       configAbs,
			Name:       displayName,
			Transport:  "stdio",
			Command:    command,
			Args:       args,
			EnvKeys:    sortedMapKeys(env),
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(displayName, "stdio", command, args, ""),
		})
	}
	return out
}
