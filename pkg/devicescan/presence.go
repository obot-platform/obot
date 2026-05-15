package devicescan

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/obot-platform/obot/apiclient/types"
)

// clientPresenceDef describes how to detect that a given AI client is
// installed on the device. Each field is a list because most clients
// have one or two canonical names; the first match wins per category.
type clientPresenceDef struct {
	binaries    []string
	appBundles  []string
	configPaths []string
}

// clientAppBundleDirs is overridable in tests so detection doesn't
// depend on the real /Applications tree. nil → platform defaults
// (/Applications and ~/Applications on darwin).
var clientAppBundleDirs []string

// detectClientPresence returns the first-matching binary, install
// path, and config path for def. Empty strings mean no signal in that
// category. Caller emits a clients[] row only if at least one is set.
func detectClientPresence(def clientPresenceDef, home string) (binary, install, configPath string) {
	for _, b := range def.binaries {
		if p, err := exec.LookPath(b); err == nil && p != "" {
			binary = p
			break
		}
	}

	if runtime.GOOS == "darwin" && len(def.appBundles) > 0 {
		bundles := clientAppBundleDirs
		if bundles == nil {
			bundles = []string{"/Applications", filepath.Join(home, "Applications")}
		}
	bundleLoop:
		for _, name := range def.appBundles {
			for _, dir := range bundles {
				candidate := filepath.Join(dir, name)
				if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
					install = candidate
					break bundleLoop
				}
			}
		}
	}

	for _, rel := range def.configPaths {
		candidate := filepath.Join(home, rel)
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			configPath = candidate
			break
		}
	}
	return
}

func DetectClaudeCodePresence(home string) types.DeviceScanClient {
	scanner := claudeCodeScanner{}
	binary, install, configPath := detectClientPresence(scanner.Presence(), home)
	return types.DeviceScanClient{
		Name:        scanner.Name(),
		BinaryPath:  binary,
		InstallPath: install,
		ConfigPath:  configPath,
	}
}

func DetectCursorPresence(home string) types.DeviceScanClient {
	scanner := cursorScanner{}
	binary, install, configPath := detectClientPresence(scanner.Presence(), home)
	return types.DeviceScanClient{
		Name:        scanner.Name(),
		BinaryPath:  binary,
		InstallPath: install,
		ConfigPath:  configPath,
	}
}

// scanClientPresence runs presence detection for every registered
// scanner and adds a clients[] row whenever any signal fires.
func scanClientPresence(s *scanState, home string) {
	for _, c := range allScanners {
		binary, install, configPath := detectClientPresence(c.Presence(), home)
		if binary == "" && install == "" && configPath == "" {
			continue
		}
		s.addClient(types.DeviceScanClient{
			Name:        c.Name(),
			BinaryPath:  binary,
			InstallPath: install,
			ConfigPath:  configPath,
		})
	}
}
