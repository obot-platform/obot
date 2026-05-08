package devicescan

import "github.com/obot-platform/obot/apiclient/types"

// ClientScanner is the per-client integration surface. Each AI client
// (Claude Code, Cursor, etc.) implements this with one struct in its own
// file. Methods are pure: they read the fs and return observations
// instead of mutating shared state.
//
// State that genuinely is shared (file table, client table) lives on
// scanState and is updated via its methods (addFile, addClient).
type ClientScanner interface {
	// Name is the wire `client` tag this scanner emits.
	Name() string

	// Presence returns the binary/app-bundle/config-dir signals used to
	// decide whether this client is installed on the device, regardless
	// of whether it has any config to scan. The orchestrator emits a
	// clients[] row when any signal fires.
	Presence() clientPresenceDef

	// GlobalConfigPaths returns fs-relative paths the scanner opens during
	// ScanGlobal. The orchestrator uses these to suppress redundant
	// project-walk hits on the same path (e.g. ~/.cursor/mcp.json appears
	// in both the global config list and the project glob).
	GlobalConfigPaths() []string

	// ProjectGlobs returns gobwas/glob patterns that match this client's
	// project-scope config files (e.g. "**/.cursor/mcp.json"). The
	// orchestrator runs a single fs.WalkDir, matches every file against
	// every scanner's globs, and dispatches hits to ScanProject.
	//
	// Empty for clients with no project-scope config.
	ProjectGlobs() []string

	// ScanGlobal opens this client's global config(s) and returns emitted
	// MCP server observations. May call s.addFile to record config files.
	ScanGlobal(s *scanState) []types.DeviceScanMCPServer

	// ScanProject parses one project-scope config file (already known to
	// match one of ProjectGlobs) and returns emitted MCP server
	// observations.
	ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer
}

// PluginScanner is implemented by clients that emit DeviceScanPlugin
// observations alongside MCP scanning. Returning slices keeps the
// orchestrator the single point of accumulation.
type PluginScanner interface {
	// ScanPlugins returns plugin observations and any nested MCP/skill
	// observations discovered alongside them.
	ScanPlugins(s *scanState) (
		plugins []types.DeviceScanPlugin,
		servers []types.DeviceScanMCPServer,
		skills []types.DeviceScanSkill,
	)
}

// allScanners is the static registry consumed by the Scan pipeline.
// Adding a new client = appending here + writing one file with a struct
// implementing ClientScanner. Order is alphabetical so emit order is
// deterministic regardless of map iteration in the orchestrator.
var allScanners = []ClientScanner{
	claudeCodeScanner{},
	claudeDesktopScanner{},
	codexScanner{},
	cursorScanner{},
	gooseScanner{},
	hermesScanner{},
	openclawScanner{},
	opencodeScanner{},
	vscodeScanner{},
	windsurfScanner{},
	zedScanner{},
}
