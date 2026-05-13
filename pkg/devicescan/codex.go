package devicescan

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"golang.org/x/mod/semver"
)

var codexConfigPath = filepath.Join(homeDir, ".codex/config.toml")

var codex = client{
	name:     "codex",
	binaries: []string{"codex"},
	directRules: []parseRule{
		{target: codexConfigPath, parse: parseCodexConfig},
		{target: filepath.Join(homeDir, ".codex/skills"), parse: parseSkillDir("codex")},
		{target: filepath.Join(homeDir, ".codex/plugins/cache"), parse: parseCodexPlugins},
	},
	walkRules: []parseRule{
		{target: ".codex/config.toml", parse: parseCodexConfig},
	},
	walkSkipPrefixes: []string{".codex/plugins", ".codex/skills"},
}

// codexConfig is Codex's TOML shape: a top-level mcp_servers map of
// named entries with Codex-specific header model.
type codexConfig struct {
	MCPServers map[string]codexEntry `toml:"mcp_servers"`
}

type codexEntry struct {
	Type              string         `toml:"type"`
	Transport         string         `toml:"transport"`
	Command           string         `toml:"command"`
	Args              []string       `toml:"args"`
	URL               string         `toml:"url"`
	Env               map[string]any `toml:"env"`
	HTTPHeaders       map[string]any `toml:"http_headers"`
	EnvHTTPHeaders    map[string]any `toml:"env_http_headers"`
	BearerTokenEnvVar string         `toml:"bearer_token_env_var"`
	Enabled           *bool          `toml:"enabled"`
}

// parseCodexConfig handles both the global ~/.codex/config.toml and
// any project-scope hit. Project rel is two levels deep inside the
// owning project (<proj>/.codex/config.toml). Emits one MCP
// observation per [mcp_servers.<name>] table. Codex header semantics:
// http_headers (literal map), env_http_headers (header_name → env_var
// name), and bearer_token_env_var (yields an implicit Authorization
// header). Only header *names* propagate.
func parseCodexConfig(path string) parseResult {
	cfg, ok := readTOML[codexConfig](path)
	if !ok {
		return parseResult{}
	}

	var (
		projectPath string
		file        = readScanFile(path)
	)
	if path != codexConfigPath {
		projectPath = filepath.Dir(filepath.Dir(path))
	}

	out := parseResult{files: []types.DeviceScanFile{file}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e           = cfg.MCPServers[name]
			headerNames = map[string]struct{}{}
		)
		if e.Enabled != nil && !*e.Enabled {
			continue
		}

		for k := range e.HTTPHeaders {
			headerNames[k] = struct{}{}
		}

		for k := range e.EnvHTTPHeaders {
			headerNames[k] = struct{}{}
		}

		if e.BearerTokenEnvVar != "" {
			headerNames["Authorization"] = struct{}{}
		}

		transport := codexTransport(e.Type, e.Transport, e.URL)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:      "codex",
			ProjectPath: projectPath,
			File:        path,
			Name:        name,
			Transport:   transport,
			Command:     e.Command,
			Args:        e.Args,
			URL:         e.URL,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  sortedMapKeys(headerNames),
			ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
		})
	}
	return out
}

// codexTransport differs from the standard rule because Codex defaults
// remote (URL-only) servers to streamable-http rather than sse.
func codexTransport(typeField, transportField, urlField string) string {
	if explicit := cmp.Or(typeField, transportField); explicit != "" {
		transport := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(explicit)), "_", "-")
		if transport == "streamablehttp" {
			transport = "streamable-http"
		}

		return transport
	}

	if urlField != "" {
		return "streamable-http"
	}

	return "stdio"
}

// parseCodexPlugins walks .codex/plugins/cache/<marketplace>/<plugin>/
// and emits a plugin observation for the highest-semver version of
// each plugin that ships a manifest at .codex-plugin/plugin.json.
func parseCodexPlugins(dirPath string) parseResult {
	marketplaces, err := os.ReadDir(dirPath)
	if err != nil {
		return parseResult{}
	}
	var out parseResult
	for _, marketplace := range marketplaces {
		if !marketplace.IsDir() {
			continue
		}

		var (
			marketplacePath = filepath.Join(dirPath, marketplace.Name())
			plugins, err    = os.ReadDir(marketplacePath)
		)
		if err != nil {
			log.Debugf("codex: skipping marketplace %q: %v", marketplace, err)
			continue
		}

		for _, p := range plugins {
			if !p.IsDir() {
				continue
			}

			var (
				pluginPath               = filepath.Join(marketplacePath, p.Name())
				versionPath, version, ok = highestVersionDir(pluginPath)
			)
			if !ok {
				continue
			}

			manifestPath := filepath.Join(versionPath, ".codex-plugin/plugin.json")
			if !fileExists(manifestPath) {
				continue
			}

			sub := parsePluginInstall(versionPath, manifestPath, pluginInstallOpts{
				client:          "codex",
				pluginType:      "codex_plugin",
				marketplace:     marketplace.Name(),
				enabled:         true,
				nameFallback:    p.Name(),
				versionFallback: version,
				nestedMCPRel:    []string{"mcp.json", ".mcp.json"},
			})
			out.merge(sub)
		}
	}
	return out
}

// highestVersionDir returns the version subdirectory with the
// highest semver key, the directory's basename (the version string),
// and ok. Non-directory entries and invalid semver names are ignored.
func highestVersionDir(pluginPath string) (string, string, bool) {
	entries, err := os.ReadDir(pluginPath)
	if err != nil {
		log.Debugf("codex: skipping plugin %q: %v", pluginPath, err)
		return "", "", false
	}

	var highestVersion, highestName string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		canonical := semver.Canonical("v" + strings.TrimPrefix(e.Name(), "v"))
		if canonical == "" {
			continue
		}

		if semver.Compare(canonical, highestVersion) > 0 {
			highestVersion = canonical
			highestName = e.Name()
		}
	}

	if highestVersion == "" {
		return "", "", false
	}

	return filepath.Join(pluginPath, highestName), highestName, true
}
