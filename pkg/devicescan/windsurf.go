package devicescan

import (
	"cmp"
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

var windsurfMCPPath = filepath.Join(homeDir, ".codeium/windsurf/mcp_config.json")

var windsurf = client{
	name:      "windsurf",
	binaries:  []string{"windsurf"},
	appBundle: "Windsurf.app",
	directRules: []parseRule{
		{target: windsurfMCPPath, parse: parseWindsurfMCP},
	},
	walkRules: []parseRule{
		{target: ".windsurf/mcp_config.json", parse: parseWindsurfMCP},
	},
}

// windsurfMCPFile is the shape of Windsurf's mcp_config.json — a
// top-level mcpServers map keyed by server name.
type windsurfMCPFile struct {
	MCPServers map[string]windsurfEntry `json:"mcpServers"`
}

// windsurfEntry extends the canonical mcpServerSpec with `serverUrl`,
// the alternate URL spelling Windsurf's docs publish for remote
// servers. Both URL and ServerURL collapse to the same logical
// endpoint at the parser call site.
type windsurfEntry struct {
	mcpServerSpec
	ServerURL string `json:"serverUrl"`
}

// parseWindsurfMCP handles both the global mcp_config.json and any
// project-scope hit. Project rel is two levels deep inside the owning
// project (<proj>/.windsurf/mcp_config.json).
func parseWindsurfMCP(path string) parseResult {
	cfg, ok := readJSON[windsurfMCPFile](path)
	if !ok {
		return parseResult{}
	}

	var projectPath string
	if path != windsurfMCPPath {
		projectPath = filepath.Dir(filepath.Dir(path))
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.MCPServers) {
		var (
			e         = cfg.MCPServers[name]
			url       = cmp.Or(e.URL, e.ServerURL)
			transport = normalizeTransport(e.Type, e.Transport, url)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:      "windsurf",
			ProjectPath: projectPath,
			File:        path,
			Name:        name,
			Transport:   transport,
			Command:     e.Command,
			Args:        e.Args,
			URL:         url,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  sortedMapKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, url),
		})
	}

	return out
}
