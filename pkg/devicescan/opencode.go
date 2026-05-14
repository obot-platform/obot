package devicescan

import (
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	opencodeGlobalConfigJSONRel  = ".config/opencode/opencode.json"
	opencodeGlobalConfigJSONCRel = ".config/opencode/opencode.jsonc"
	opencodeLocalPluginsRel      = ".config/opencode/plugins"
	opencodeNPMCacheRel          = ".cache/opencode/node_modules"
)

var opencodePluginExts = map[string]bool{
	".js":  true,
	".ts":  true,
	".mjs": true,
	".mts": true,
}

// opencodeConfig is opencode.json's shape: top-level `mcp` map of named
// entries, plus `plugin` array listing npm package plugin names.
type opencodeConfig struct {
	MCP    map[string]opencodeEntry `json:"mcp"`
	Plugin []string                 `json:"plugin"`
}

// opencodeEntry has OpenCode-specific transport tags ("local"/"remote")
// and a Command-as-array shape for stdio.
type opencodeEntry struct {
	Type        string         `json:"type"`
	Command     []string       `json:"command"`
	URL         string         `json:"url"`
	Environment map[string]any `json:"environment"`
	Headers     map[string]any `json:"headers"`
	Enabled     *bool          `json:"enabled"`
}

type opencodeScanner struct{}

func (opencodeScanner) Name() string { return "opencode" }

func (opencodeScanner) Presence() clientPresenceDef {
	return clientPresenceDef{binaries: []string{"opencode"}, configPaths: []string{".config/opencode"}}
}

func (opencodeScanner) GlobalConfigPaths() []string {
	return []string{opencodeGlobalConfigJSONRel, opencodeGlobalConfigJSONCRel}
}

func (opencodeScanner) ProjectGlobs() []string { return []string{"**/opencode.json"} }

func (opencodeScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	var out []types.DeviceScanMCPServer
	for _, rel := range []string{opencodeGlobalConfigJSONRel, opencodeGlobalConfigJSONCRel} {
		cfg, ok := readJSON[opencodeConfig](s.fsys, rel)
		if !ok {
			continue
		}
		configAbs := s.addFileOrAbs(rel)
		out = append(out, opencodeEmit(cfg.MCP, configAbs, "")...)
	}
	return out
}

func (opencodeScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[opencodeConfig](s.fsys, configRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(configRel)
	projectAbs := s.abs(path.Dir(configRel))
	return opencodeEmit(cfg.MCP, configAbs, projectAbs)
}

func opencodeEmit(servers map[string]opencodeEntry, configAbs, projectAbs string) []types.DeviceScanMCPServer {
	out := make([]types.DeviceScanMCPServer, 0, len(servers))
	for name, e := range servers {
		if e.Enabled != nil && !*e.Enabled {
			continue
		}
		obs, ok := e.toServer(name, configAbs, projectAbs)
		if !ok {
			continue
		}
		out = append(out, obs)
	}
	return out
}

func (e opencodeEntry) toServer(name, configAbs, projectAbs string) (types.DeviceScanMCPServer, bool) {
	switch e.Type {
	case "local":
		if len(e.Command) == 0 {
			return types.DeviceScanMCPServer{}, false
		}
		cmd := e.Command[0]
		var args []string
		if len(e.Command) > 1 {
			args = e.Command[1:]
		}
		return types.DeviceScanMCPServer{
			Client:      "opencode",
			ProjectPath: projectAbs,
			File:        configAbs,
			Name:        name,
			Transport:   "stdio",
			Command:     cmd,
			Args:        args,
			EnvKeys:     sortedMapKeys(e.Environment),
			HeaderKeys:  []string{},
			ConfigHash:  mcpConfigHash(name, "stdio", cmd, args, ""),
		}, true
	case "remote":
		if e.URL == "" {
			return types.DeviceScanMCPServer{}, false
		}
		return types.DeviceScanMCPServer{
			Client:      "opencode",
			ProjectPath: projectAbs,
			File:        configAbs,
			Name:        name,
			Transport:   "http",
			URL:         e.URL,
			EnvKeys:     []string{},
			HeaderKeys:  sortedMapKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, "http", "", nil, e.URL),
		}, true
	}
	return types.DeviceScanMCPServer{}, false
}

// ScanPlugins emits Plugin observations for OpenCode plugins. Two
// sources: subdirectories under ~/.config/opencode/plugins/, and npm
// packages listed under opencode.json's `plugin` array, found in
// ~/.cache/opencode/node_modules/<pkg>/.
func (opencodeScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	ps1, ms1, sks1 := scanOpenCodeLocalPlugins(s)
	ps2, ms2, sks2 := scanOpenCodeNPMPlugins(s)
	return append(ps1, ps2...), append(ms1, ms2...), append(sks1, sks2...)
}

func scanOpenCodeLocalPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	entries, err := fs.ReadDir(s.fsys, opencodeLocalPluginsRel)
	if err != nil {
		return nil, nil, nil
	}
	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for _, e := range entries {
		itemRel := path.Join(opencodeLocalPluginsRel, e.Name())
		if e.IsDir() {
			ep := emitOpenCodePluginDir(s, itemRel, e.Name(), "opencode_plugin", "")
			ps = append(ps, ep.plugin)
			ms = append(ms, ep.servers...)
			sks = append(sks, ep.skills...)
			continue
		}
		if !opencodePluginExts[path.Ext(e.Name())] {
			continue
		}
		// Standalone plugin file.
		fileAbs, err := s.addFile(itemRel)
		if err != nil {
			log.Debugf("opencode: skipping plugin file %q: %v", itemRel, err)
			continue
		}
		base := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		ps = append(ps, types.DeviceScanPlugin{
			Client:     "opencode",
			ConfigPath: fileAbs,
			Name:       base,
			PluginType: "opencode_plugin",
			Enabled:    true,
			Files:      []string{fileAbs},
			HasHooks:   true,
		})
	}
	return ps, ms, sks
}

func scanOpenCodeNPMPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	names := readOpenCodeNPMPluginNames(s.fsys, opencodeGlobalConfigJSONRel)
	for _, n := range readOpenCodeNPMPluginNames(s.fsys, opencodeGlobalConfigJSONCRel) {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, nil, nil
	}
	if !dirExists(s.fsys, opencodeNPMCacheRel) {
		return nil, nil, nil
	}
	var (
		ps  []types.DeviceScanPlugin
		ms  []types.DeviceScanMCPServer
		sks []types.DeviceScanSkill
	)
	for _, pkg := range names {
		pkgRel := path.Join(opencodeNPMCacheRel, pkg)
		if !dirExists(s.fsys, pkgRel) {
			continue
		}
		ep := emitOpenCodePluginDir(s, pkgRel, pkg, "opencode_npm_plugin", "npm")
		ps = append(ps, ep.plugin)
		ms = append(ms, ep.servers...)
		sks = append(sks, ep.skills...)
	}
	return ps, ms, sks
}

// emitOpenCodePluginDir reads a plugin directory's package.json (if any)
// for metadata and produces an emittedPlugin. Nested MCP config is looked
// up at {mcp.json, .mcp.json}.
func emitOpenCodePluginDir(s *scanState, installRel, fallbackName, pluginType, marketplace string) emittedPlugin {
	packageRel := path.Join(installRel, "package.json")
	pkg, _ := readJSON[map[string]any](s.fsys, packageRel)
	name, version, description, author := manifestMetadata(pkg)
	if name == "" {
		name = fallbackName
	}

	supporting := s.listArtifactPaths(installRel, pluginExts)

	mcpRaw, mcpSourceRel := pluginMCPServersBlock(s.fsys, installRel, []string{"mcp.json", ".mcp.json"}, pkg, packageRel)
	mcpSourceAbs := ""
	if mcpSourceRel != "" {
		mcpSourceAbs = s.addFileOrAbs(mcpSourceRel)
	}
	pluginFileAbs := ""
	if pkg != nil {
		pluginFileAbs = s.addFileOrAbs(packageRel)
	}

	var servers []types.DeviceScanMCPServer
	for serverName, raw := range mcpRaw {
		entry, ok := decodeMCPServerSpec(raw)
		if !ok {
			continue
		}
		servers = append(servers, entry.toServer(serverName, "opencode", mcpSourceAbs, ""))
	}

	return emittedPlugin{
		plugin: types.DeviceScanPlugin{
			Client:        "opencode",
			ConfigPath:    pluginFileAbs,
			Name:          name,
			PluginType:    pluginType,
			Version:       version,
			Description:   description,
			Author:        author,
			Enabled:       true,
			Marketplace:   marketplace,
			Files:         supporting,
			HasMCPServers: len(mcpRaw) > 0,
			HasHooks:      true,
		},
		servers: servers,
	}
}

func readOpenCodeNPMPluginNames(fsys fs.FS, rel string) []string {
	cfg, ok := readJSON[opencodeConfig](fsys, rel)
	if !ok {
		return nil
	}
	return cfg.Plugin
}
