package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
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

func TestSetupNonInteractiveDetectedInstallsCursor(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	root := setupTestRoot(func(_ context.Context, _ string, _, _ bool) (string, error) {
		return "token", nil
	})
	setup := &Setup{
		URL:    "https://obot.example.com/",
		Agents: "detected",
		Yes:    true,
		root:   root,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := setupTestCommand(nil, &stdout, &stderr)
	if err := setup.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}

	skillPath := filepath.Join(home, ".cursor", "skills", "obot", skillformat.SkillMainFile)
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "rendered for `cursor`") {
		t.Fatalf("unexpected bootstrap content:\n%s", content)
	}
	if !strings.Contains(stdout.String(), "Installed Obot bootstrap skills for Cursor") {
		t.Fatalf("expected install message, got stdout:\n%s", stdout.String())
	}
}

func TestSetupExplicitCursor(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0755); err != nil {
		t.Fatal(err)
	}

	root := setupTestRoot(func(_ context.Context, _ string, _, _ bool) (string, error) {
		return "token", nil
	})
	setup := &Setup{
		URL:    "https://obot.example.com/",
		Agents: "cursor",
		Yes:    true,
		root:   root,
	}

	if err := setup.Run(setupTestCommand(nil, nil, nil), nil); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, filepath.Join(home, ".cursor", "skills", "obot", skillformat.SkillMainFile), "rendered for `cursor`")
}

func TestSetupNonInteractiveMissingURLFailsWithoutPrompt(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	root := setupTestRoot(func(context.Context, string, bool, bool) (string, error) {
		t.Fatal("token fetcher should not be called without a URL")
		return "", nil
	})
	setup := &Setup{
		Agents:         "detected",
		NonInteractive: true,
		root:           root,
	}

	var stdout bytes.Buffer
	cmd := setupTestCommand(strings.NewReader("https://obot.example.com\n"), &stdout, nil)
	err := setup.Run(cmd, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--url is required in non-interactive mode") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(stdout.String(), "Obot URL:") {
		t.Fatalf("non-interactive setup should not prompt, got stdout:\n%s", stdout.String())
	}
}

func TestSetupAgentsNoneSkipsLocalAgentInstall(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	root := setupTestRoot(func(_ context.Context, _ string, _, _ bool) (string, error) {
		return "token", nil
	})
	setup := &Setup{
		URL:            "https://obot.example.com/",
		Agents:         "none",
		Yes:            true,
		NonInteractive: true,
		root:           root,
	}

	var stdout, stderr bytes.Buffer
	cmd := setupTestCommand(nil, &stdout, &stderr)
	if err := setup.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(stdout.String(), "Detected:") {
		t.Fatalf("expected agent detection to be skipped, got stdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Skipping local agent bootstrap installation") {
		t.Fatalf("expected skip message, got stdout:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "obot", skillformat.SkillMainFile)); !os.IsNotExist(err) {
		t.Fatalf("expected no Claude Code skill to be installed, stat err: %v", err)
	}
}

func TestSetupJSONProgressSuccessfulSequence(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	root := setupTestRoot(func(_ context.Context, _ string, _, _ bool) (string, error) {
		return "token", nil
	})
	setup := &Setup{
		URL:            "https://obot.example.com/",
		Agents:         "detected",
		Yes:            true,
		NonInteractive: true,
		Output:         "json",
		root:           root,
	}

	var stdout, stderr bytes.Buffer
	cmd := setupTestCommand(nil, &stdout, &stderr)
	if err := setup.Run(cmd, nil); err != nil {
		t.Fatal(err)
	}

	events := setupProgressEvents(t, stdout.Bytes())
	gotTypes := make([]string, 0, len(events))
	for _, event := range events {
		gotTypes = append(gotTypes, event.Type)
	}
	wantTypes := []string{"auth_started", "auth_completed", "config_saved", "agent_installed", "complete"}
	if strings.Join(gotTypes, ",") != strings.Join(wantTypes, ",") {
		t.Fatalf("event types = %v, want %v\nstdout:\n%s", gotTypes, wantTypes, stdout.String())
	}
	if events[3].AgentID != localagents.ClaudeCodeAgentID {
		t.Fatalf("agent_installed agentID = %q, want %q", events[3].AgentID, localagents.ClaudeCodeAgentID)
	}
	if events[3].DisplayName != "Claude Code" {
		t.Fatalf("agent_installed displayName = %q, want Claude Code", events[3].DisplayName)
	}
	if len(events[3].Installed) == 0 {
		t.Fatalf("agent_installed should include installed paths")
	}
	if strings.Contains(stdout.String(), "Detected:") {
		t.Fatalf("JSON stdout should not include human setup output:\n%s", stdout.String())
	}
	if strings.Contains(stderr.String(), "Logged in to") {
		t.Fatalf("JSON stderr should not include routine login status, got:\n%s", stderr.String())
	}
}

func TestSetupJSONProgressStructuredError(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	root := setupTestRoot(func(context.Context, string, bool, bool) (string, error) {
		t.Fatal("token fetcher should not be called without a URL")
		return "", nil
	})
	setup := &Setup{
		Agents:         "none",
		NonInteractive: true,
		Output:         "json",
		root:           root,
	}

	var stdout bytes.Buffer
	cmd := setupTestCommand(strings.NewReader("https://obot.example.com\n"), &stdout, nil)
	err := setup.Run(cmd, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !ErrorAlreadyReported(err) {
		t.Fatalf("JSON setup errors should be marked already reported, got %T: %v", err, err)
	}

	events := setupProgressEvents(t, stdout.Bytes())
	if len(events) != 1 {
		t.Fatalf("expected one error event, got %#v\nstdout:\n%s", events, stdout.String())
	}
	if events[0].Type != "error" {
		t.Fatalf("event type = %q, want error", events[0].Type)
	}
	if events[0].Code != "invalid_url" {
		t.Fatalf("event code = %q, want invalid_url", events[0].Code)
	}
	if !strings.Contains(events[0].Message, "--url is required in non-interactive mode") {
		t.Fatalf("unexpected error message: %q", events[0].Message)
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

func TestParseSetupAgentsNoneIsExclusive(t *testing.T) {
	_, err := parseSetupAgents("none,cursor")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetupStatusJSONNoConfig(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()

	status := &SetupStatus{
		JSON: true,
		tokenValid: func(context.Context, string) (bool, error) {
			t.Fatal("token validator should not run without a configured URL")
			return false, nil
		},
	}

	var stdout bytes.Buffer
	if err := status.Run(setupTestCommand(nil, &stdout, nil), nil); err != nil {
		t.Fatal(err)
	}

	var got setupStatusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("status output should be JSON: %v\n%s", err, stdout.String())
	}
	if got.Version == "" {
		t.Fatalf("expected version in status: %#v", got)
	}
	if got.DefaultURL != "" {
		t.Fatalf("defaultURL = %q, want empty", got.DefaultURL)
	}
	if got.TokenValid {
		t.Fatalf("tokenValid = true, want false")
	}
	if got.SetupComplete {
		t.Fatalf("setupComplete = true, want false")
	}
	if !strings.Contains(stdout.String(), `"defaultURL": ""`) {
		t.Fatalf("status JSON should include empty defaultURL, got:\n%s", stdout.String())
	}
}

func TestSetupStatusSetupCompleteRequiresConfiguredURLAndValidToken(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	if err := localconfig.Save(localconfig.Config{DefaultURL: "https://obot.example.com/"}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		tokenValid    bool
		setupComplete bool
	}{
		{name: "valid token", tokenValid: true, setupComplete: true},
		{name: "invalid token", tokenValid: false, setupComplete: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &SetupStatus{
				JSON: true,
				tokenValid: func(_ context.Context, appURL string) (bool, error) {
					if appURL != "https://obot.example.com" {
						t.Fatalf("appURL = %q, want normalized URL", appURL)
					}
					return tt.tokenValid, nil
				},
			}

			var stdout bytes.Buffer
			if err := status.Run(setupTestCommand(nil, &stdout, nil), nil); err != nil {
				t.Fatal(err)
			}

			var got setupStatusOutput
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("status output should be JSON: %v\n%s", err, stdout.String())
			}
			if got.DefaultURL != "https://obot.example.com" {
				t.Fatalf("defaultURL = %q, want normalized configured URL", got.DefaultURL)
			}
			if got.TokenValid != tt.tokenValid {
				t.Fatalf("tokenValid = %t, want %t", got.TokenValid, tt.tokenValid)
			}
			if got.SetupComplete != tt.setupComplete {
				t.Fatalf("setupComplete = %t, want %t", got.SetupComplete, tt.setupComplete)
			}
		})
	}
}

func TestSetupDetectAgentsJSON(t *testing.T) {
	restore := useRootTestEnv(t)
	defer restore()
	home := useSetupTestHome(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	detect := &SetupDetectAgents{JSON: true}
	if err := detect.Run(setupTestCommand(nil, &stdout, nil), nil); err != nil {
		t.Fatal(err)
	}

	var got setupDetectAgentsOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("detect-agents output should be JSON: %v\n%s", err, stdout.String())
	}
	if len(got.Agents) != 2 {
		t.Fatalf("expected two agents, got %#v", got.Agents)
	}
	if got.Agents[0].ID != localagents.ClaudeCodeAgentID {
		t.Fatalf("first agent id = %q, want %q", got.Agents[0].ID, localagents.ClaudeCodeAgentID)
	}
	if got.Agents[0].DisplayName != "Claude Code" {
		t.Fatalf("first agent displayName = %q, want Claude Code", got.Agents[0].DisplayName)
	}
	if got.Agents[0].State != string(localagents.DetectionPresent) {
		t.Fatalf("first agent state = %q, want present; reason: %s", got.Agents[0].State, got.Agents[0].Reason)
	}
	if got.Agents[0].Reason == "" {
		t.Fatalf("expected reason for first agent")
	}
	if got.Agents[1].ID != localagents.CursorAgentID {
		t.Fatalf("second agent id = %q, want %q", got.Agents[1].ID, localagents.CursorAgentID)
	}
	if got.Agents[1].DisplayName != "Cursor" {
		t.Fatalf("second agent displayName = %q, want Cursor", got.Agents[1].DisplayName)
	}
	if got.Agents[1].State != string(localagents.DetectionMissing) {
		t.Fatalf("second agent state = %q, want missing; reason: %s", got.Agents[1].State, got.Agents[1].Reason)
	}
	if got.Agents[1].Reason == "" {
		t.Fatalf("expected reason for second agent")
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

func setupProgressEvents(t *testing.T, data []byte) []setupProgressEvent {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	var events []setupProgressEvent
	for {
		var event setupProgressEvent
		err := dec.Decode(&event)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("setup progress output should be newline-delimited JSON: %v\n%s", err, string(data))
		}
		events = append(events, event)
	}
	return events
}
