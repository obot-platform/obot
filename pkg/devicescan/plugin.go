package devicescan

import (
	"cmp"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

// pluginExts is the file-collection extension allowlist used when
// ingesting plugin install directories. Wider than skillExts because
// plugins commonly include manifest, schema, and config files
// alongside scripts.
var pluginExts = map[string]struct{}{
	".md":    {},
	".mdc":   {},
	".txt":   {},
	".sh":    {},
	".py":    {},
	".js":    {},
	".ts":    {},
	".json":  {},
	".jsonc": {},
	".yaml":  {},
	".yml":   {},
	".toml":  {},
}

// artifactSkipDirs are dependency / build directories we never descend
// into when walking inside a skill or plugin directory.
var artifactSkipDirs = map[string]struct{}{
	"node_modules": {},
	".venv":        {},
	"venv":         {},
	"vendor":       {},
	"dist":         {},
	".tox":         {},
	".git":         {},
	"__pycache__":  {},
}

// pluginInstallOpts is the per-host configuration for
// parsePluginInstall. Each plugin host (claude_code, cursor, codex,
// opencode) supplies its own client/pluginType/marketplace and the
// set of candidate nested MCP filenames; the parsing routine itself
// is identical.
type pluginInstallOpts struct {
	client       string
	pluginType   string
	marketplace  string
	enabled      bool
	nameFallback string
	// versionFallback is used when the manifest is missing or omits
	// version (e.g. Claude Code carries it on the registry entry, not
	// the per-plugin manifest).
	versionFallback string
	// nestedMCPRel is the list of basenames under installPath that may
	// carry the plugin's nested MCP server definitions. First match
	// wins.
	nestedMCPRel []string
	// mcpServerXform, when non-nil, is invoked on each nested MCP
	// server's parsed spec before conversion (used by Claude Code for
	// ${CLAUDE_PLUGIN_ROOT} substitution).
	mcpServerXform func(*mcpServerSpec)
}

// pluginManifest is the union of fields we read from a plugin's
// manifest file. Different plugin hosts use different manifest
// filenames; the shape happens to be the same.
type pluginManifest struct {
	Name        string                    `json:"name"`
	Version     string                    `json:"version"`
	Description string                    `json:"description"`
	Author      pluginAuthor              `json:"author"`
	MCPServers  map[string]pluginMCPEntry `json:"mcpServers"`
}

// pluginMCPEntry extends the canonical mcpServerSpec with `serverUrl`,
// the alternate URL spelling some plugin manifests carry alongside
// `url`. The parser collapses both into the single wire URL field.
type pluginMCPEntry struct {
	mcpServerSpec
	ServerURL string `json:"serverUrl"`
}

// pluginAuthor wraps the author field's two valid shapes: a bare
// string, or an object with a "name" field.
type pluginAuthor struct{ Name string }

func (p *pluginAuthor) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		p.Name = s
		return nil
	}
	var obj struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	p.Name = obj.Name
	return nil
}

// listArtifactPaths walks dirPath and returns abs paths of files with
// an allowed extension, skipping artifactSkipDirs. Path-only — the
// bytes are not uploaded.
func listArtifactPaths(dirPath string, allowedExts map[string]struct{}) []string {
	var paths []string
	_ = filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if _, skip := artifactSkipDirs[d.Name()]; path != dirPath && skip {
				return filepath.SkipDir
			}
			return nil
		}
		if _, ok := allowedExts[filepath.Ext(path)]; !ok {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	return paths
}

// parsePluginInstall reads one plugin's install dir and emits the
// plugin observation plus any nested MCP servers and skills it
// defines. opts supplies per-host config (client tag, manifest
// filenames, optional MCP server transform).
func parsePluginInstall(installPath, manifestPath string, opts pluginInstallOpts) parseResult {
	var (
		manifest, _ = readJSON[pluginManifest](manifestPath)
		out         = parseResult{files: []types.DeviceScanFile{readScanFile(manifestPath)}}
		name        = cmp.Or(manifest.Name, opts.nameFallback)
		version     = cmp.Or(manifest.Version, opts.versionFallback)
		// has_* rollups: mcp checks the canonical top-level files; the
		// rest are dir-existence checks. hasMCP may flip on later if
		// nested servers are discovered.
		hasMCP = fileExists(filepath.Join(installPath, "mcp.json")) ||
			fileExists(filepath.Join(installPath, ".mcp.json"))
		hasSkills                             = dirExists(filepath.Join(installPath, "skills"))
		hasRules                              = dirExists(filepath.Join(installPath, "rules"))
		hasCommands                           = dirExists(filepath.Join(installPath, "commands"))
		hasHooks                              = dirExists(filepath.Join(installPath, "hooks"))
		servers, mcpSourcePath, mcpSourceFile = readPluginNestedMCP(installPath, opts.nestedMCPRel)
	)

	if len(servers) > 0 {
		if mcpSourceFile.Path != "" {
			out.files = append(out.files, mcpSourceFile)
		}
	} else if len(manifest.MCPServers) > 0 {
		mcpSourcePath = manifestPath
		servers = manifest.MCPServers
	}
	if len(servers) > 0 {
		hasMCP = true
	}

	for _, serverName := range sortedMapKeys(servers) {
		var (
			entry = servers[serverName]
			spec  = entry.mcpServerSpec
		)
		if spec.URL == "" {
			spec.URL = entry.ServerURL
		}

		if opts.mcpServerXform != nil {
			opts.mcpServerXform(&spec)
		}

		transport := normalizeTransport(spec.Type, spec.Transport, spec.URL)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:     opts.client,
			File:       mcpSourcePath,
			Name:       serverName,
			Transport:  transport,
			Command:    spec.Command,
			Args:       spec.Args,
			URL:        spec.URL,
			EnvKeys:    sortedMapKeys(spec.Env),
			HeaderKeys: sortedMapKeys(spec.Headers),
			ConfigHash: mcpConfigHash(serverName, transport, spec.Command, spec.Args, spec.URL),
		})
	}

	if hasSkills {
		var (
			skillsRoot   = filepath.Join(installPath, "skills")
			entries, err = os.ReadDir(skillsRoot)
		)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}

				skillDir := filepath.Join(skillsRoot, e.Name())
				if !fileExists(filepath.Join(skillDir, "SKILL.md")) {
					continue
				}

				sub, ok := parseSkill(skillDir, opts.client, "")
				if !ok {
					continue
				}
				out.merge(sub)
			}
		}
	}

	out.plugins = append(out.plugins, types.DeviceScanPlugin{
		Client:        opts.client,
		ConfigPath:    manifestPath,
		Name:          name,
		PluginType:    opts.pluginType,
		Version:       version,
		Description:   manifest.Description,
		Author:        manifest.Author.Name,
		Enabled:       opts.enabled,
		Marketplace:   opts.marketplace,
		Files:         listArtifactPaths(installPath, pluginExts),
		HasMCPServers: hasMCP,
		HasSkills:     hasSkills,
		HasRules:      hasRules,
		HasCommands:   hasCommands,
		HasHooks:      hasHooks,
	})
	return out
}

// readPluginNestedMCP returns the first non-empty mcpServers map
// found among candidate basenames under installPath, plus the source
// file's abs path and DeviceScanFile record. As a permissive fallback,
// a candidate file whose top-level entries look like server specs
// (carry command / url / serverUrl) is treated as a root-keyed map.
func readPluginNestedMCP(installPath string, candidates []string) (map[string]pluginMCPEntry, string, types.DeviceScanFile) {
	for _, fname := range candidates {
		filePath := filepath.Join(installPath, fname)
		// Try the canonical {mcpServers: {...}} shape first.
		nested, ok := readJSON[struct {
			MCPServers map[string]pluginMCPEntry `json:"mcpServers"`
		}](filePath)
		if ok && len(nested.MCPServers) > 0 {
			return nested.MCPServers, filePath, readScanFile(filePath)
		}
		// Fallback: top-level map of server-spec-shaped entries.
		root, ok := readJSON[map[string]pluginMCPEntry](filePath)
		if !ok {
			continue
		}
		filtered := map[string]pluginMCPEntry{}
		for k, v := range root {
			if v.Command != "" || v.URL != "" || v.ServerURL != "" {
				filtered[k] = v
			}
		}
		if len(filtered) > 0 {
			return filtered, filePath, readScanFile(filePath)
		}
	}
	return nil, "", types.DeviceScanFile{}
}
