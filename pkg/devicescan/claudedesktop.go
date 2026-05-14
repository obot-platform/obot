package devicescan

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// Claude Desktop on-disk layout (macOS only — Windows %APPDATA% is not
// reachable through os.DirFS rooted at $HOME, so the file simply will
// not exist there).
const (
	claudeDesktopExtRel    = "Library/Application Support/Claude/extensions-installations.json"
	claudeDesktopConfigRel = "Library/Application Support/Claude/claude_desktop_config.json"
)

// claudeDesktopExtensions wraps the top-level `extensions` map in
// extensions-installations.json. Each extension carries a manifest with
// nested server info.
type claudeDesktopExtensions struct {
	Extensions map[string]struct {
		Manifest struct {
			DisplayName string `json:"display_name"`
			Server      struct {
				Type       string         `json:"type"`
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
		configPaths: []string{"Library/Application Support/Claude", ".config/Claude"},
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

// ScanPlugins emits one DeviceScanPlugin (plugin_type =
// claude_desktop_connector) per entry in the mcpServers block of
// claude_desktop_config.json. The MCP server itself is already produced
// by ScanGlobal — the plugin entry captures the connector as a
// first-class artifact alongside.
func (claudeDesktopScanner) ScanPlugins(s *scanState) (
	[]types.DeviceScanPlugin, []types.DeviceScanMCPServer, []types.DeviceScanSkill,
) {
	cfg, ok := readJSON[claudeDesktopConfig](s.fsys, claudeDesktopConfigRel)
	if !ok || len(cfg.MCPServers) == 0 {
		return nil, nil, nil
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
	return out, nil, nil
}

func scanClaudeDesktopExtensions(s *scanState) []types.DeviceScanMCPServer {
	cfg, ok := readJSON[claudeDesktopExtensions](s.fsys, claudeDesktopExtRel)
	if !ok {
		return nil
	}
	configAbs := s.addFileOrAbs(claudeDesktopExtRel)

	out := make([]types.DeviceScanMCPServer, 0, len(cfg.Extensions))
	for name, ext := range cfg.Extensions {
		out = append(out, claudeDesktopExtensionToServer(name, ext, configAbs))
	}
	return out
}

func claudeDesktopExtensionToServer(name string, ext struct {
	Manifest struct {
		DisplayName string `json:"display_name"`
		Server      struct {
			Type       string         `json:"type"`
			MCPConfig  *mcpServerSpec `json:"mcp_config"`
			EntryPoint string         `json:"entry_point"`
		} `json:"server"`
	} `json:"manifest"`
}, configAbs string) types.DeviceScanMCPServer {
	displayName := name
	if ext.Manifest.DisplayName != "" {
		displayName = ext.Manifest.DisplayName
	}

	transport := ext.Manifest.Server.Type
	if transport == "" {
		transport = "stdio"
	}
	if transport == "node" {
		transport = "stdio"
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

	return types.DeviceScanMCPServer{
		Client:     "claude_desktop",
		File:       configAbs,
		Name:       displayName,
		Transport:  transport,
		Command:    command,
		Args:       args,
		EnvKeys:    sortedMapKeys(env),
		HeaderKeys: []string{},
		ConfigHash: mcpConfigHash(displayName, transport, command, args, ""),
	}
}
