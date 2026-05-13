package devicescan

import (
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

var claudeDesktop = client{
	name:      "claude_desktop",
	appBundle: "Claude.app",
	directRules: []parseRule{
		{target: filepath.Join(configDir, "Claude/extensions-installations.json"), parse: parseClaudeDesktopExtensions},
		{target: filepath.Join(configDir, "Claude/claude_desktop_config.json"), parse: parseClaudeDesktopConfig},
	},
}

// claudeDesktopExtensions wraps the top-level `extensions` map in
// extensions-installations.json. Each extension carries a manifest
// with nested server info.
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

// claudeDesktopConfig is the shape of claude_desktop_config.json,
// used both for MCP server scanning and for connector-plugin
// enumeration.
type claudeDesktopConfig struct {
	MCPServers map[string]mcpServerSpec `json:"mcpServers"`
}

func parseClaudeDesktopExtensions(path string) parseResult {
	cfg, ok := readJSON[claudeDesktopExtensions](path)
	if !ok {
		return parseResult{}
	}
	file := readScanFile(path)
	out := parseResult{files: []types.DeviceScanFile{file}}
	for _, name := range sortedMapKeys(cfg.Extensions) {
		ext := cfg.Extensions[name]
		displayName := name
		if ext.Manifest.DisplayName != "" {
			displayName = ext.Manifest.DisplayName
		}

		// Claude Desktop manifests use "node" or "stdio" — both surface
		// as stdio on the wire. Otherwise carry the literal type.
		transport := ext.Manifest.Server.Type
		if transport == "" || transport == "node" {
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

		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:     "claude_desktop",
			File:       path,
			Name:       displayName,
			Transport:  transport,
			Command:    command,
			Args:       args,
			EnvKeys:    sortedMapKeys(env),
			ConfigHash: mcpConfigHash(displayName, transport, command, args, ""),
		})
	}
	return out
}

// parseClaudeDesktopConfig emits MCP server rows from the
// `mcpServers` block of claude_desktop_config.json AND one plugin
// observation per server (plugin_type = claude_desktop_connector) so
// the connector itself shows up as a first-class artefact alongside
// the underlying MCP server.
func parseClaudeDesktopConfig(path string) parseResult {
	cfg, ok := readJSON[claudeDesktopConfig](path)
	if !ok {
		return parseResult{}
	}

	var (
		file = readScanFile(path)
		out  = parseResult{files: []types.DeviceScanFile{file}}
	)
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e         = cfg.MCPServers[name]
			transport = normalizeTransport(e.Type, e.Transport, e.URL)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:     "claude_desktop",
			File:       path,
			Name:       name,
			Transport:  transport,
			Command:    e.Command,
			Args:       e.Args,
			URL:        e.URL,
			EnvKeys:    sortedMapKeys(e.Env),
			HeaderKeys: sortedMapKeys(e.Headers),
			ConfigHash: mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
		})
		out.plugins = append(out.plugins, types.DeviceScanPlugin{
			Client:        "claude_desktop",
			ConfigPath:    path,
			Name:          name,
			PluginType:    "claude_desktop_connector",
			Enabled:       e.Enabled == nil || *e.Enabled,
			Files:         []string{path},
			HasMCPServers: true,
		})
	}

	return out
}
