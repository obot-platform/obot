package devicescan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var opencodePluginExts = map[string]struct{}{
	".js":  {},
	".ts":  {},
	".mjs": {},
	".mts": {},
}

var (
	opencodeJSONPath  = filepath.Join(homeDir, ".config/opencode/opencode.json")
	opencodeJSONCPath = filepath.Join(homeDir, ".config/opencode/opencode.jsonc")
)

var opencode = client{
	name:     "opencode",
	binaries: []string{"opencode"},
	directRules: []parseRule{
		{target: opencodeJSONPath, parse: parseOpencodeConfig},
		{target: opencodeJSONCPath, parse: parseOpencodeConfig},
		{target: filepath.Join(homeDir, ".config/opencode/skills"), parse: parseSkillDir("opencode")},
		{target: filepath.Join(homeDir, ".config/opencode/plugins"), parse: parseOpencodeLocalPlugins},
		{target: filepath.Join(homeDir, ".cache/opencode/node_modules"), parse: parseOpencodeNPMPlugins},
	},
	walkRules: []parseRule{
		{target: "opencode.json", parse: parseOpencodeConfig},
	},
	walkSkipPrefixes: []string{".config/opencode/plugins", ".config/opencode/skills"},
}

// opencodeConfig is opencode.json's shape: a top-level mcp map of
// named entries plus a `plugin` array of npm package names.
type opencodeConfig struct {
	MCP    map[string]opencodeEntry `json:"mcp"`
	Plugin []string                 `json:"plugin"`
}

// opencodeEntry has OpenCode-specific transport tags ("local" /
// "remote") and a Command-as-array shape for stdio.
type opencodeEntry struct {
	Type        string         `json:"type"`
	Command     []string       `json:"command"`
	URL         string         `json:"url"`
	Environment map[string]any `json:"environment"`
	Headers     map[string]any `json:"headers"`
	Enabled     *bool          `json:"enabled"`
}

// parseOpencodeConfig handles both global opencode.{json,jsonc} and
// any project-scope opencode.json. Project rel is one level deep
// inside the owning project (<proj>/opencode.json). Emits one MCP
// observation per mcp entry: "local" (stdio with command-as-array) or
// "remote" (http with URL); other types are dropped.
func parseOpencodeConfig(path string) parseResult {
	cfg, ok := readJSON[opencodeConfig](path)
	if !ok {
		return parseResult{}
	}
	file := readScanFile(path)
	var projectPath string
	if path != opencodeJSONPath && path != opencodeJSONCPath {
		projectPath = filepath.Dir(path)
	}
	out := parseResult{files: []types.DeviceScanFile{file}}
	for _, name := range sortedMapKeys(cfg.MCP) {
		e := cfg.MCP[name]
		if e.Enabled != nil && !*e.Enabled {
			continue
		}

		switch e.Type {
		case "local":
			if len(e.Command) == 0 {
				continue
			}
			cmd := e.Command[0]
			var args []string
			if len(e.Command) > 1 {
				args = e.Command[1:]
			}
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:      "opencode",
				ProjectPath: projectPath,
				File:        path,
				Name:        name,
				Transport:   "stdio",
				Command:     cmd,
				Args:        args,
				EnvKeys:     sortedMapKeys(e.Environment),
				ConfigHash:  mcpConfigHash(name, "stdio", cmd, args, ""),
			})
		case "remote":
			if e.URL == "" {
				continue
			}
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:      "opencode",
				ProjectPath: projectPath,
				File:        path,
				Name:        name,
				Transport:   "http",
				URL:         e.URL,
				HeaderKeys:  sortedMapKeys(e.Headers),
				ConfigHash:  mcpConfigHash(name, "http", "", nil, e.URL),
			})
		}
	}
	return out
}

// parseOpencodeLocalPlugins enumerates ~/.config/opencode/plugins/.
// Subdirectories become package-style plugins; standalone .js/.ts
// files become file-style plugins.
func parseOpencodeLocalPlugins(dirPath string) parseResult {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return parseResult{}
	}
	var out parseResult
	for _, e := range entries {
		itemPath := filepath.Join(dirPath, e.Name())
		if e.IsDir() {
			sub := emitOpencodePluginDir(itemPath, e.Name(), "opencode_plugin", "")
			out.merge(sub)
			continue
		}
		if _, ok := opencodePluginExts[filepath.Ext(e.Name())]; !ok {
			continue
		}
		file := readScanFile(itemPath)
		if file.Path == "" {
			log.Debugf("opencode: skipping plugin file %q", itemPath)
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		out.files = append(out.files, file)
		out.plugins = append(out.plugins, types.DeviceScanPlugin{
			Client:     "opencode",
			ConfigPath: itemPath,
			Name:       base,
			PluginType: "opencode_plugin",
			Enabled:    true,
			Files:      []string{itemPath},
			HasHooks:   true,
		})
	}
	return out
}

// parseOpencodeNPMPlugins reads npm package names listed under the
// `plugin` array of either opencode.json or opencode.jsonc, then
// looks each one up under ~/.cache/opencode/node_modules/<pkg>/.
func parseOpencodeNPMPlugins(dirPath string) parseResult {
	names := readOpencodeNPMPluginNames(opencodeJSONPath)
	for _, n := range readOpencodeNPMPluginNames(opencodeJSONCPath) {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return parseResult{}
	}
	var out parseResult
	for _, pkg := range names {
		pkgPath := filepath.Join(dirPath, pkg)
		if !dirExists(pkgPath) {
			continue
		}
		sub := emitOpencodePluginDir(pkgPath, pkg, "opencode_npm_plugin", "npm")
		out.merge(sub)
	}
	return out
}

// emitOpencodePluginDir reads one plugin directory and emits its
// observation. OpenCode plugin manifests live in package.json
// (NPM-style), which the shared parsePluginInstall already handles
// generically — but OpenCode's "plugin" notion doesn't quite align
// with the other clients' (everything is a hook host), so the wire
// HasHooks rollup is forced true here.
func emitOpencodePluginDir(installPath, fallbackName, pluginType, marketplace string) parseResult {
	manifestPath := filepath.Join(installPath, "package.json")
	if !fileExists(manifestPath) {
		// No manifest — emit a stub plugin row keyed on the dir name.
		return parseResult{
			plugins: []types.DeviceScanPlugin{{
				Client:     "opencode",
				Name:       fallbackName,
				PluginType: pluginType,
				Enabled:    true,
				Files:      listArtifactPaths(installPath, pluginExts),
				HasHooks:   true,
			}},
		}
	}

	sub := parsePluginInstall(installPath, manifestPath, pluginInstallOpts{
		client:       "opencode",
		pluginType:   pluginType,
		marketplace:  marketplace,
		enabled:      true,
		nameFallback: fallbackName,
		nestedMCPRel: []string{"mcp.json", ".mcp.json"},
	})

	// override: OpenCode plugins are unconditional hook hosts (see docstring).
	for i := range sub.plugins {
		sub.plugins[i].HasHooks = true
	}

	return sub
}

func readOpencodeNPMPluginNames(path string) []string {
	cfg, ok := readJSON[opencodeConfig](path)
	if !ok {
		return nil
	}

	return cfg.Plugin
}
