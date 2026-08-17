package system

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Bin returns the path to the current Obot executable.
func Bin() string {
	if bin := os.Getenv("OBOT_BIN"); bin != "" {
		return bin
	}
	return currentBin()
}

func currentBin() string {
	name := os.Args[0]
	if filepath.Base(name) == name {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	if absoluteName, err := filepath.Abs(name); err == nil {
		return absoluteName
	}
	return name
}
