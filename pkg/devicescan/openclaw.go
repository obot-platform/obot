package devicescan

import (
	"os"

	"github.com/obot-platform/obot/apiclient/types"
)

// openclawScanner is presence-only: OpenClaw has no public config or
// plugin format we scan today. Its config directory varies with
// $OPENCLAW_PROFILE, so it owns that resolution rather than leaking a
// special case into shared presence code.
type openclawScanner struct{}

func (openclawScanner) Name() string { return "openclaw" }

func (openclawScanner) Presence() clientPresenceDef {
	configPath := ".openclaw"
	if profile := os.Getenv("OPENCLAW_PROFILE"); profile != "" {
		configPath = ".openclaw-" + profile
	}
	return clientPresenceDef{
		binaries:    []string{"openclaw"},
		appBundles:  []string{"OpenClaw.app"},
		configPaths: []string{configPath},
	}
}

func (openclawScanner) GlobalConfigPaths() []string { return nil }

func (openclawScanner) ProjectGlobs() []string { return nil }

func (openclawScanner) ScanGlobal(*scanState) []types.DeviceScanMCPServer { return nil }

func (openclawScanner) ScanProject(*scanState, string) []types.DeviceScanMCPServer { return nil }
