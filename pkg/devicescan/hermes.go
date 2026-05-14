package devicescan

import "github.com/obot-platform/obot/apiclient/types"

const hermesGlobalConfigRel = ".hermes/config.yaml"

// hermesConfig is Hermes's config.yaml shape: a single top-level
// `mcp_servers` map of named entries.
type hermesConfig struct {
	MCPServers map[string]hermesEntry `yaml:"mcp_servers"`
}

// hermesEntry mirrors mcpServerSpec but with Hermes-specific transport
// rules: `url` implies streamable-http (no explicit type field), and
// `enabled` defaults to true (only honoured when explicitly false).
type hermesEntry struct {
	Command string         `yaml:"command"`
	Args    []string       `yaml:"args"`
	URL     string         `yaml:"url"`
	Env     map[string]any `yaml:"env"`
	Headers map[string]any `yaml:"headers"`
	Enabled *bool          `yaml:"enabled"`
}

type hermesScanner struct{}

func (hermesScanner) Name() string { return "hermes" }

func (hermesScanner) Presence() clientPresenceDef {
	return clientPresenceDef{binaries: []string{"hermes"}, configPaths: []string{".hermes"}}
}

func (hermesScanner) GlobalConfigPaths() []string { return []string{hermesGlobalConfigRel} }

func (hermesScanner) ProjectGlobs() []string { return nil } // global config only

func (hermesScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readYAML[hermesConfig](s.fsys, hermesGlobalConfigRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(hermesGlobalConfigRel)

	out := make([]types.DeviceScanMCPServer, 0, len(cfg.MCPServers))
	for name, e := range cfg.MCPServers {
		if e.Enabled != nil && !*e.Enabled {
			continue
		}
		obs, ok := e.toServer(name, configAbs)
		if !ok {
			continue
		}
		out = append(out, obs)
	}
	return out
}

func (hermesScanner) ScanProject(*scanState, string) []types.DeviceScanMCPServer { return nil }

// toServer materialises a Hermes entry. Returns ok=false for entries
// with neither command nor url (settings-only stubs).
func (e hermesEntry) toServer(name, configAbs string) (types.DeviceScanMCPServer, bool) {
	if e.Command != "" {
		return types.DeviceScanMCPServer{
			Client:     "hermes",
			File:       configAbs,
			Name:       name,
			Transport:  "stdio",
			Command:    e.Command,
			Args:       e.Args,
			EnvKeys:    sortedMapKeys(e.Env),
			HeaderKeys: []string{},
			ConfigHash: mcpConfigHash(name, "stdio", e.Command, e.Args, ""),
		}, true
	}
	if e.URL != "" {
		return types.DeviceScanMCPServer{
			Client:     "hermes",
			File:       configAbs,
			Name:       name,
			Transport:  "streamable-http",
			URL:        e.URL,
			EnvKeys:    sortedMapKeys(e.Env),
			HeaderKeys: sortedMapKeys(e.Headers),
			ConfigHash: mcpConfigHash(name, "streamable-http", "", nil, e.URL),
		}, true
	}
	return types.DeviceScanMCPServer{}, false
}
