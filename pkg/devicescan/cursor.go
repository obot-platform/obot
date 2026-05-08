package devicescan

import (
	"io/fs"
	"path"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	cursorGlobalConfigRel   = ".cursor/mcp.json"
	cursorSettingsRel       = ".cursor/settings.json"
	cursorPluginCacheRel    = ".cursor/plugins/cache/cursor-public"
	cursorPluginManifestSub = ".cursor-plugin/plugin.json"
	cursorMarketplace       = "cursor-public"
)

type cursorScanner struct{}

func (cursorScanner) Name() string { return "cursor" }

func (cursorScanner) Presence() clientPresenceDef {
	return clientPresenceDef{
		binaries:    []string{"cursor"},
		appBundles:  []string{"Cursor.app"},
		configPaths: []string{".cursor"},
	}
}

func (cursorScanner) GlobalConfigPaths() []string { return []string{cursorGlobalConfigRel} }

func (cursorScanner) ProjectGlobs() []string { return []string{"**/.cursor/mcp.json"} }

func (cursorScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	return emitJSONServersGlobal(s, cursorGlobalConfigRel, "mcpServers", "cursor")
}

func (cursorScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	projectAbs := s.abs(path.Dir(path.Dir(configRel)))
	return emitJSONServersProject(s, configRel, "mcpServers", "cursor", projectAbs)
}

// ScanPlugins walks ~/.cursor/plugins/cache/cursor-public/<name>/<hash>/
// for .cursor-plugin/plugin.json manifests. Dedupes by plugin name (first
// hash dir wins) and resolves enabled state from .cursor/settings.json
// (two key forms: "<name>@cursor-public" and "<name>").
func (cursorScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	plugins, err := fs.ReadDir(s.fsys, cursorPluginCacheRel)
	if err != nil {
		return nil, nil, nil
	}
	enabledByKey := readEnabledPluginsMap(s.fsys, cursorSettingsRel)
	seen := map[string]bool{}

	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for _, p := range plugins {
		if !p.IsDir() || seen[p.Name()] {
			continue
		}
		pluginRel := path.Join(cursorPluginCacheRel, p.Name())
		hashes, err := fs.ReadDir(s.fsys, pluginRel)
		if err != nil {
			log.Debugf("cursor: skipping plugin %q: %v", pluginRel, err)
			continue
		}
		for _, h := range hashes {
			if !h.IsDir() {
				continue
			}
			installRel := path.Join(pluginRel, h.Name())
			manifestRel := path.Join(installRel, cursorPluginManifestSub)
			if !fileExists(s.fsys, manifestRel) {
				continue
			}

			enabled := false
			for _, key := range []string{p.Name() + "@" + cursorMarketplace, p.Name()} {
				if v, ok := enabledByKey[key]; ok {
					enabled = v
					break
				}
			}

			ep := emitPlugin(s, emitPluginOpts{
				installRel:   installRel,
				manifestRel:  manifestRel,
				pluginType:   "cursor_plugin",
				client:       "cursor",
				marketplace:  cursorMarketplace,
				enabled:      enabled,
				nameFallback: p.Name(),
				nestedMCPRel: []string{"mcp.json", ".mcp.json"},
			})
			ps = append(ps, ep.plugin)
			ms = append(ms, ep.servers...)
			sks = append(sks, ep.skills...)
			seen[p.Name()] = true
			break
		}
	}
	return ps, ms, sks
}
