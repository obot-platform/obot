package devicescan

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const gooseGlobalConfigRel = ".config/goose/config.yaml"

// gooseConfig is Goose's config.yaml shape: a top-level `extensions` map.
// Goose uses non-standard field names (cmd/envs/uri instead of
// command/env/url) and gates every entry on a required `enabled: true`.
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

type gooseScanner struct{}

func (gooseScanner) Name() string { return "goose" }

func (gooseScanner) Presence() clientPresenceDef {
	return clientPresenceDef{binaries: []string{"goose"}, configPaths: []string{".config/goose"}}
}

func (gooseScanner) GlobalConfigPaths() []string { return []string{gooseGlobalConfigRel} }

func (gooseScanner) ProjectGlobs() []string { return nil }

func (gooseScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readYAML[gooseConfig](s.fsys, gooseGlobalConfigRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(gooseGlobalConfigRel)

	out := make([]types.DeviceScanMCPServer, 0, len(cfg.Extensions))
	for key, ext := range cfg.Extensions {
		if !ext.Enabled {
			continue
		}
		obs, ok := ext.toServer(key, configAbs)
		if !ok {
			continue
		}
		out = append(out, obs)
	}
	return out
}

func (gooseScanner) ScanProject(*scanState, string) []types.DeviceScanMCPServer { return nil }

// toServer materialises a Goose extension. Only stdio/sse/streamable_http
// types are surfaced (other types are MCP-irrelevant).
func (e gooseExtension) toServer(key, configAbs string) (types.DeviceScanMCPServer, bool) {
	switch e.Type {
	case "stdio", "sse", "streamable_http":
	default:
		return types.DeviceScanMCPServer{}, false
	}
	name := key
	if e.Name != "" {
		name = e.Name
	}

	if e.Type == "stdio" {
		return types.DeviceScanMCPServer{
			Client:     "goose",
			File:       configAbs,
			Name:       name,
			Transport:  "stdio",
			Command:    e.Cmd,
			Args:       e.Args,
			EnvKeys:    sortedMapKeys(e.Envs),
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(name, "stdio", e.Cmd, e.Args, ""),
		}, true
	}

	transport := strings.ReplaceAll(e.Type, "_", "-")
	return types.DeviceScanMCPServer{
		Client:     "goose",
		File:       configAbs,
		Name:       name,
		Transport:  transport,
		URL:        e.URI,
		EnvKeys:    sortedMapKeys(e.Envs),
		HeaderKeys: sortedMapKeys(e.Headers),
		ConfigHash: mcpConfigHash(name, transport, "", nil, e.URI),
	}, true
}
