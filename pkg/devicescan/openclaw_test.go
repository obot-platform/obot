package devicescan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// withCleanPath sets PATH to a single directory, optionally containing
// a stub `openclaw` executable. Returns the directory used.
func withCleanPath(t *testing.T, dir string, withStub bool) string {
	t.Helper()
	if dir == "" {
		t.Setenv("PATH", "")
		return ""
	}
	if withStub {
		stub := filepath.Join(dir, "openclaw")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatalf("write stub: %v", err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

// runScan exercises scanClientPresence against a fresh state and
// returns the openclaw row if emitted (nil otherwise).
func runScan(t *testing.T, home string) *types.DeviceScanClient {
	t.Helper()
	s := newScanState(nil, home)
	scanClientPresence(s, home)
	if c, ok := s.clients["openclaw"]; ok {
		out := c
		return &out
	}
	return nil
}

func TestScanOpenClaw_NotInstalled(t *testing.T) {
	home := t.TempDir()
	withCleanPath(t, t.TempDir(), false)
	t.Setenv("OPENCLAW_PROFILE", "")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	if c := runScan(t, home); c != nil {
		t.Fatalf("expected no client emitted, got %+v", c)
	}
}

func TestScanOpenClaw_PathOnly(t *testing.T) {
	home := t.TempDir()
	pathDir := withCleanPath(t, t.TempDir(), true)
	t.Setenv("OPENCLAW_PROFILE", "")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	c := runScan(t, home)
	if c == nil {
		t.Fatalf("expected client emitted")
	}
	wantBin := filepath.Join(pathDir, "openclaw")
	if c.BinaryPath != wantBin {
		t.Fatalf("BinaryPath = %q, want %q", c.BinaryPath, wantBin)
	}
	if c.InstallPath != "" || c.ConfigPath != "" {
		t.Fatalf("expected only BinaryPath set, got %+v", c)
	}
}

func TestScanOpenClaw_StateDir(t *testing.T) {
	home := t.TempDir()
	withCleanPath(t, t.TempDir(), false)
	t.Setenv("OPENCLAW_PROFILE", "")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	stateDir := filepath.Join(home, ".openclaw")
	if err := os.Mkdir(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	c := runScan(t, home)
	if c == nil {
		t.Fatalf("expected detected client")
	}
	if c.ConfigPath != stateDir {
		t.Fatalf("ConfigPath = %q, want %q", c.ConfigPath, stateDir)
	}
}

func TestScanOpenClaw_StateDirWithProfile(t *testing.T) {
	home := t.TempDir()
	withCleanPath(t, t.TempDir(), false)
	t.Setenv("OPENCLAW_PROFILE", "dev")
	clientAppBundleDirs = []string{t.TempDir()}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	profileDir := filepath.Join(home, ".openclaw-dev")
	if err := os.Mkdir(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	// Distractor: unsuffixed dir must NOT match when profile is set.
	if err := os.Mkdir(filepath.Join(home, ".openclaw"), 0o755); err != nil {
		t.Fatalf("mkdir distractor: %v", err)
	}

	c := runScan(t, home)
	if c == nil {
		t.Fatalf("expected detected client")
	}
	if c.ConfigPath != profileDir {
		t.Fatalf("ConfigPath = %q, want %q", c.ConfigPath, profileDir)
	}
}

func TestScanOpenClaw_AppBundleDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("app bundle check only runs on darwin")
	}
	home := t.TempDir()
	withCleanPath(t, t.TempDir(), false)
	t.Setenv("OPENCLAW_PROFILE", "")

	bundleParent := t.TempDir()
	bundle := filepath.Join(bundleParent, "OpenClaw.app")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	clientAppBundleDirs = []string{bundleParent}
	t.Cleanup(func() { clientAppBundleDirs = nil })

	c := runScan(t, home)
	if c == nil {
		t.Fatalf("expected detected client")
	}
	if c.InstallPath != bundle {
		t.Fatalf("InstallPath = %q, want %q", c.InstallPath, bundle)
	}
}
