package devicescan

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/obot-platform/obot/apiclient/types"
)

// detectPresence runs the OS signal checks for c and, if any fire,
// returns the wire DeviceScanClient row plus true. Real-OS access —
// $PATH lookup, /Applications stat (darwin only), and config-path
// existence checks. Returns (zero, false) for clients with no signals
// or with an empty name.
func detectPresence(c client) (types.DeviceScanClient, bool) {
	if c.name == "" {
		return types.DeviceScanClient{}, false
	}

	var binary string
	for _, b := range c.binaries {
		if p, err := exec.LookPath(b); err == nil && p != "" {
			binary = p
			break
		}
	}

	var install string
	if runtime.GOOS == "darwin" && c.appBundle != "" {
		for _, dir := range []string{macAppsDir, filepath.Join(homeDir, "Applications")} {
			candidate := filepath.Join(dir, c.appBundle)
			if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
				install = candidate
				break
			}
		}
	}

	// configPath: first directRules target that exists. Provides a
	// presence signal for GUI apps that aren't in $PATH (e.g. Claude
	// Desktop on Linux).
	var configPath string
	for _, r := range c.directRules {
		if r.target == "" {
			continue
		}

		if _, err := os.Stat(r.target); err == nil {
			configPath = r.target
			break
		}
	}

	if binary == "" && install == "" && configPath == "" {
		return types.DeviceScanClient{}, false
	}

	return types.DeviceScanClient{
		Name:        c.name,
		BinaryPath:  binary,
		InstallPath: install,
		ConfigPath:  configPath,
	}, true
}
