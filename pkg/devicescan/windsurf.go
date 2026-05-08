package devicescan

import (
	"path"

	"github.com/obot-platform/obot/apiclient/types"
)

const windsurfGlobalConfigRel = ".codeium/windsurf/mcp_config.json"

type windsurfScanner struct{}

func (windsurfScanner) Name() string { return "windsurf" }

func (windsurfScanner) Presence() clientPresenceDef {
	return clientPresenceDef{
		binaries:    []string{"windsurf"},
		appBundles:  []string{"Windsurf.app"},
		configPaths: []string{".windsurf", ".codeium"},
	}
}

func (windsurfScanner) GlobalConfigPaths() []string { return []string{windsurfGlobalConfigRel} }

func (windsurfScanner) ProjectGlobs() []string {
	return []string{"**/.windsurf/mcp_config.json"}
}

func (windsurfScanner) ScanGlobal(s *scanState) []types.DeviceScanMCPServer {
	return emitJSONServersGlobal(s, windsurfGlobalConfigRel, "mcpServers", "windsurf")
}

func (windsurfScanner) ScanProject(s *scanState, configRel string) []types.DeviceScanMCPServer {
	projectAbs := s.abs(path.Dir(path.Dir(configRel)))
	return emitJSONServersProject(s, configRel, "mcpServers", "windsurf", projectAbs)
}
