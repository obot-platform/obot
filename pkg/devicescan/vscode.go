package devicescan

import (
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
)

var vscodeMCPPath = filepath.Join(configDir, "Code/User/mcp.json")

var vscode = client{
	name:      "vscode",
	binaries:  []string{"code"},
	appBundle: "Visual Studio Code.app",
	directRules: []parseRule{
		{target: vscodeMCPPath, parse: parseVSCodeMCP},
	},
	walkRules: []parseRule{
		{target: ".vscode/mcp.json", parse: parseVSCodeMCP},
	},
}

// vscodeMCPFile is the shape of VS Code's mcp.json — note VS Code
// uses "servers" rather than "mcpServers" for the top-level key.
type vscodeMCPFile struct {
	Servers map[string]mcpServerSpec `json:"servers"`
}

// parseVSCodeMCP handles both the global mcp.json and any project-scope
// hit. Project rel is two levels deep inside the owning project
// (<proj>/.vscode/mcp.json).
func parseVSCodeMCP(path string) parseResult {
	cfg, ok := readJSON[vscodeMCPFile](path)
	if !ok {
		return parseResult{}
	}

	var projectPath string
	if path != vscodeMCPPath {
		projectPath = filepath.Dir(filepath.Dir(path))
	}

	out := parseResult{files: []types.DeviceScanFile{readScanFile(path)}}
	for _, name := range sortedMapKeys(cfg.Servers) {
		var (
			e         = cfg.Servers[name]
			transport = normalizeTransport(e.Type, e.Transport, e.URL)
		)
		out.mcps = append(out.mcps, types.DeviceScanMCPServer{
			Client:      "vscode",
			ProjectPath: projectPath,
			File:        path,
			Name:        name,
			Transport:   transport,
			Command:     e.Command,
			Args:        e.Args,
			URL:         e.URL,
			EnvKeys:     sortedMapKeys(e.Env),
			HeaderKeys:  sortedMapKeys(e.Headers),
			ConfigHash:  mcpConfigHash(name, transport, e.Command, e.Args, e.URL),
		})
	}
	return out
}
