package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
)

func TestNewClientUsesEnvOverrides(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	if err := localconfig.Save(localconfig.Config{DefaultURL: "https://stored.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("OBOT_BASE_URL", "https://env.example.com/api"); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("OBOT_TOKEN", "env-token"); err != nil {
		t.Fatal(err)
	}

	client := newClient()
	if client.BaseURL != "https://env.example.com/api" {
		t.Fatalf("expected env base URL, got %q", client.BaseURL)
	}
	if client.Token != "env-token" {
		t.Fatalf("expected env token, got %q", client.Token)
	}
}

func TestNewClientUsesConfiguredDefaultURL(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	if err := localconfig.Save(localconfig.Config{DefaultURL: "https://stored.example.com/"}); err != nil {
		t.Fatal(err)
	}

	client := newClient()
	if client.BaseURL != "https://stored.example.com/api" {
		t.Fatalf("expected configured base URL, got %q", client.BaseURL)
	}
}

func TestNewClientFallsBackToLocalhost(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	client := newClient()
	if client.BaseURL != "http://localhost:8080/api" {
		t.Fatalf("expected localhost base URL, got %q", client.BaseURL)
	}
}

func TestNewClientWarnsOnInvalidConfig(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	configPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "obot", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	var baseURL string
	stderr := captureStderr(t, func() {
		client := newClient()
		baseURL = client.BaseURL
	})
	if baseURL != "http://localhost:8080/api" {
		t.Fatalf("expected localhost fallback, got %q", baseURL)
	}
	if !strings.Contains(stderr, "Warning: failed to load Obot config:") {
		t.Fatalf("expected config warning, got %q", stderr)
	}
}

func useRootTestEnv(t *testing.T) func() {
	t.Helper()

	configHome := filepath.Join(t.TempDir(), "config")
	oldConfigHome, hadConfigHome := os.LookupEnv("XDG_CONFIG_HOME")
	oldBaseURL, hadBaseURL := os.LookupEnv("OBOT_BASE_URL")
	oldToken, hadToken := os.LookupEnv("OBOT_TOKEN")

	if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
		t.Fatal(err)
	}
	_ = os.Unsetenv("OBOT_BASE_URL")
	_ = os.Unsetenv("OBOT_TOKEN")
	xdg.Reload()

	return func() {
		if hadConfigHome {
			_ = os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		if hadBaseURL {
			_ = os.Setenv("OBOT_BASE_URL", oldBaseURL)
		} else {
			_ = os.Unsetenv("OBOT_BASE_URL")
		}
		if hadToken {
			_ = os.Setenv("OBOT_TOKEN", oldToken)
		} else {
			_ = os.Unsetenv("OBOT_TOKEN")
		}
		xdg.Reload()
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writeFile
	defer func() {
		os.Stderr = oldStderr
	}()

	fn()

	if err := writeFile.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(readFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := readFile.Close(); err != nil {
		t.Fatal(err)
	}
	return string(data)
}
