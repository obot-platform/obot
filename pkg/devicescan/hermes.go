package devicescan

import (
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

var hermes = client{
	name:     "hermes",
	binaries: []string{"hermes"},
	directRules: []parseRule{
		{target: filepath.Join(homeDir, ".hermes/config.yaml"), parse: parseHermesGlobalConfig},
	},
}

// hermesConfig is Hermes's config.yaml shape: a single top-level
// mcp_servers map of named entries.
type hermesConfig struct {
	MCPServers map[string]hermesEntry `yaml:"mcp_servers"`
}

// hermesEntry mirrors the canonical wire fields with Hermes-specific
// transport rules: `url` implies streamable-http (no explicit type
// field), and `enabled` defaults to true (only honoured when
// explicitly false).
type hermesEntry struct {
	Command string         `yaml:"command"`
	Args    []string       `yaml:"args"`
	URL     string         `yaml:"url"`
	Env     map[string]any `yaml:"env"`
	Headers map[string]any `yaml:"headers"`
	Enabled *bool          `yaml:"enabled"`
}

// parseHermesGlobalConfig emits one MCP observation per mcp_servers
// entry. Command → stdio; URL → streamable-http; neither → drop.
func parseHermesGlobalConfig(path string) parseResult {
	cfg, ok := readYAML[hermesConfig](path)
	if !ok {
		return parseResult{}
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		e := cfg.MCPServers[name]
		if e.Enabled != nil && !*e.Enabled {
			continue
		}

		switch {
		case e.Command != "":
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:     "hermes",
				File:       path,
				Name:       name,
				Transport:  "stdio",
				Command:    e.Command,
				Args:       e.Args,
				EnvKeys:    sortedMapKeys(e.Env),
				ConfigHash: mcpConfigHash(name, "stdio", e.Command, e.Args, ""),
			})
		case e.URL != "":
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:     "hermes",
				File:       path,
				Name:       name,
				Transport:  "streamable-http",
				URL:        e.URL,
				HeaderKeys: sortedMapKeys(e.Headers),
				ConfigHash: mcpConfigHash(name, "streamable-http", "", nil, e.URL),
			})
		}
	}
	return out
}
