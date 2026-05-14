package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/skillformat"
	"github.com/spf13/cobra"
)

func TestSetupNonInteractiveDetectedInstallsClaudeCode(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	var tokenBaseURL string
	root := setupTestRoot(func(_ context.Context, baseURL string, _, _ bool) (string, error) {
		tokenBaseURL = baseURL
		return "token", nil
	})
	setup := &Setup{
		URL:    "https://obot.example.com/",
		Agents: "detected",
		Yes:    true,
		root:   root,
	}

	var stdout, stderr bytes.Buffer
	cmd := setupTestCommand(nil, &stdout, &stderr)
	if err := setup.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}

	cfg, err := localconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultURL != "https://obot.example.com" {
		t.Fatalf("DefaultURL = %q, want normalized URL", cfg.DefaultURL)
	}
	if tokenBaseURL != "https://obot.example.com/api" {
		t.Fatalf("token base URL = %q, want API URL", tokenBaseURL)
	}

	skillPath := filepath.Join(home, ".claude", "skills", "obot", skillformat.SkillMainFile)
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "rendered for `claude-code`") {
		t.Fatalf("unexpected bootstrap content:\n%s", content)
	}
	if !strings.Contains(stdout.String(), "Installed Obot bootstrap skills for Claude Code") {
		t.Fatalf("expected install message, got stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Logged in to https://obot.example.com") {
		t.Fatalf("expected login message, got stderr:\n%s", stderr.String())
	}
}

func TestSetupRefusesToReplaceConfiguredURLWithoutYes(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	if err := localconfig.Save(localconfig.Config{DefaultURL: "https://old.example.com"}); err != nil {
		t.Fatal(err)
	}

	setup := &Setup{
		URL:    "https://new.example.com",
		Agents: "detected",
		root:   setupTestRoot(nil),
	}
	err := setup.Run(setupTestCommand(nil, nil, nil), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "pass --yes to replace") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetupInteractiveUsesConfiguredURL(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := localconfig.Save(localconfig.Config{DefaultURL: "https://stored.example.com/"}); err != nil {
		t.Fatal(err)
	}

	var tokenBaseURL string
	root := setupTestRoot(func(_ context.Context, baseURL string, _, _ bool) (string, error) {
		tokenBaseURL = baseURL
		return "token", nil
	})
	setup := &Setup{
		Agents: "claude-code",
		root:   root,
	}

	var stdout bytes.Buffer
	cmd := setupTestCommand(strings.NewReader("y\n"), &stdout, nil)
	if err := setup.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}

	if tokenBaseURL != "https://stored.example.com/api" {
		t.Fatalf("token base URL = %q, want stored API URL", tokenBaseURL)
	}
	if !strings.Contains(stdout.String(), "Use this URL?") {
		t.Fatalf("expected confirmation prompt, got stdout:\n%s", stdout.String())
	}
}

func TestParseSetupAgentsRejectsAll(t *testing.T) {
	_, err := parseSetupAgents("all")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported --agents value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewIncludesSetupCommand(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	root := New()
	if _, _, err := root.Find([]string{"setup"}); err != nil {
		t.Fatalf("setup command was not registered: %v", err)
	}
}

func setupTestRoot(fetcher func(context.Context, string, bool, bool) (string, error)) *Obot {
	client := &apiclient.Client{}
	if fetcher != nil {
		client = client.WithTokenFetcher(fetcher)
	}
	return &Obot{Client: client}
}

func setupTestCommand(stdin *strings.Reader, stdout, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	if stdin != nil {
		cmd.SetIn(stdin)
	}
	if stdout != nil {
		cmd.SetOut(stdout)
	}
	if stderr != nil {
		cmd.SetErr(stderr)
	}
	return cmd
}

func useSetupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	oldHome, hadHome := os.LookupEnv("HOME")
	oldPath, hadPath := os.LookupEnv("PATH")
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("PATH", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if hadPath {
			_ = os.Setenv("PATH", oldPath)
		} else {
			_ = os.Unsetenv("PATH")
		}
	})
	return home
}
