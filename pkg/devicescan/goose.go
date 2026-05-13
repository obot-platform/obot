package devicescan

import (
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var goose = client{
	name:     "goose",
	binaries: []string{"goose"},
	directRules: []parseRule{
		{target: filepath.Join(homeDir, ".config/goose/config.yaml"), parse: parseGooseGlobalConfig},
	},
}

// gooseConfig is Goose's config.yaml shape: a top-level extensions
// map. Goose uses non-standard field names (cmd/envs/uri instead of
// command/env/url).
type gooseConfig struct {
	Extensions map[string]gooseExtension `yaml:"extensions"`
}

type gooseExtension struct {
	Type    string         `yaml:"type"`
	Name    string         `yaml:"name"`
	Cmd     string         `yaml:"cmd"`
	Args    []string       `yaml:"args"`
	URI     string         `yaml:"uri"`
	Envs    map[string]any `yaml:"envs"`
	Headers map[string]any `yaml:"headers"`
	Enabled bool           `yaml:"enabled"`
}

// parseGooseGlobalConfig emits one MCP observation per extension. Only
// stdio/sse/streamable_http types are surfaced; other types are
// MCP-irrelevant and dropped.
func parseGooseGlobalConfig(path string) parseResult {
	cfg, ok := readYAML[gooseConfig](path)
	if !ok {
		return parseResult{}
	}

	var (
		file = readScanFile(path)
		out  = parseResult{files: []types.DeviceScanFile{file}}
	)
	for _, key := range sortedMapKeys(cfg.Extensions) {
		var (
			e    = cfg.Extensions[key]
			name = key
		)
		if !e.Enabled {
			continue
		}

		if e.Name != "" {
			name = e.Name
		}

		switch e.Type {
		case "stdio":
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:     "goose",
				File:       path,
				Name:       name,
				Transport:  "stdio",
				Command:    e.Cmd,
				Args:       e.Args,
				EnvKeys:    sortedMapKeys(e.Envs),
				ConfigHash: mcpConfigHash(name, "stdio", e.Cmd, e.Args, ""),
			})
		case "sse", "streamable_http":
			transport := strings.ReplaceAll(e.Type, "_", "-")
			out.mcps = append(out.mcps, types.DeviceScanMCPServer{
				Client:     "goose",
				File:       path,
				Name:       name,
				Transport:  transport,
				URL:        e.URI,
				HeaderKeys: sortedMapKeys(e.Headers),
				ConfigHash: mcpConfigHash(name, transport, "", nil, e.URI),
			})
		}
	}
	return out
}
