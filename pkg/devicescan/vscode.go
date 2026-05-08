package devicescan

import (
	"path"

	"github.com/obot-platform/obot/apiclient/types"
)

// VS Code on-disk layout. The macOS path is relative to $HOME; on other
// platforms the file simply will not exist and ScanGlobal is a no-op.
const (
	vscodeGlobalConfigRel = "Library/Application Support/Code/User/mcp.json"
)

type vscodeScanner struct{}

func (vscodeScanner) Name() string { return "vscode" }

func (vscodeScanner) Presence() clientPresenceDef {
	return clientPresenceDef{
		binaries:    []string{"code"},
		appBundles:  []string{"Visual Studio Code.app"},
		configPaths: []string{".vscode", "Library/Application Support/Code"},
	}
}

func (vscodeScanner) GlobalConfigPaths() []string { return []string{vscodeGlobalConfigRel} }

func (vscodeScanner) ProjectGlobs() []string { return []string{"**/.vscode/mcp.json"} }

// VS Code uses "servers" rather than "mcpServers" for both global and
// project configs; entries follow the standard JSON shape.
func (vscodeScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	return emitJSONServersGlobal(s, vscodeGlobalConfigRel, "servers", "vscode")
}

func (vscodeScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	projectAbs := s.abs(path.Dir(path.Dir(configRel)))
	return emitJSONServersProject(s, configRel, "servers", "vscode", projectAbs)
}
