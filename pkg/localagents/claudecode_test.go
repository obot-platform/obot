package localagents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/skillformat"
)

func TestClaudeCodeDetectPresentFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	result := ClaudeCode{home: home}.Detect(context.Background())
	if result.State != DetectionPresent {
		t.Fatalf("State = %q, want %q; reason: %s", result.State, DetectionPresent, result.Reason)
	}
	if !strings.Contains(result.Reason, ".claude") {
		t.Fatalf("Reason = %q, want config path", result.Reason)
	}
}

func TestClaudeCodeDetectMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	result := ClaudeCode{home: t.TempDir()}.Detect(context.Background())
	if result.State != DetectionMissing {
		t.Fatalf("State = %q, want %q; reason: %s", result.State, DetectionMissing, result.Reason)
	}
}

func TestClaudeCodeInstallBootstrapWritesExpectedSkills(t *testing.T) {
	home := t.TempDir()

	result, err := NewClaudeCode().InstallBootstrap(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}

	if result.AgentID != ClaudeCodeAgentID {
		t.Fatalf("AgentID = %q, want %q", result.AgentID, ClaudeCodeAgentID)
	}
	if len(result.Installed) != 4 {
		t.Fatalf("Installed count = %d, want 4: %#v", len(result.Installed), result.Installed)
	}

	for _, name := range []string{"obot", "obot-search-skills", "obot-install-skill", "obot-scan"} {
		content := readFile(t, filepath.Join(home, ".claude", "skills", name, skillformat.SkillMainFile))
		if !strings.Contains(content, "Obot") && !strings.Contains(content, "obot") {
			t.Fatalf("%s content did not look like an Obot bootstrap skill:\n%s", name, content)
		}
	}
}

func TestClaudeCodeInstallBootstrapOverwritesExistingContent(t *testing.T) {
	home := t.TempDir()
	oldSkill := filepath.Join(home, ".claude", "skills", "obot", skillformat.SkillMainFile)
	if err := os.MkdirAll(filepath.Dir(oldSkill), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldSkill, []byte("old local edit"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewClaudeCode().InstallBootstrap(context.Background(), home); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, oldSkill)
	if strings.Contains(content, "old local edit") {
		t.Fatalf("bootstrap install preserved old content:\n%s", content)
	}
	if !strings.Contains(content, "rendered for `claude-code`") {
		t.Fatalf("bootstrap content was not replaced with rendered asset:\n%s", content)
	}
}

func TestClaudeCodeInstallSkillWritesSanitizedDirectory(t *testing.T) {
	home := t.TempDir()
	skill := SkillArchive{
		Name: "GitHub Review!",
		Files: []SkillArchiveFile{
			{
				RelPath: skillformat.SkillMainFile,
				Content: []byte("---\nname: github-review\ndescription: Review GitHub changes.\n---\nBody\n"),
			},
			{
				RelPath: "scripts/check.sh",
				Content: []byte("#!/bin/sh\nexit 0\n"),
				Mode:    0755,
			},
		},
	}

	result, err := NewClaudeCode().InstallSkill(context.Background(), home, skill)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(home, ".claude", "skills", "github-review")
	assertFileContains(t, filepath.Join(target, skillformat.SkillMainFile), "Review GitHub changes")
	assertFileContains(t, filepath.Join(target, "scripts", "check.sh"), "exit 0")
	if len(result.Installed) != 2 {
		t.Fatalf("Installed count = %d, want 2", len(result.Installed))
	}

	info, err := os.Stat(filepath.Join(target, "scripts", "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("script mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestClaudeCodeInstallSkillReplacesExistingTarget(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".claude", "skills", "github-review")
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "stale.txt"), []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewClaudeCode().InstallSkill(context.Background(), home, SkillArchive{
		Name: "github-review",
		Files: []SkillArchiveFile{
			{
				RelPath: skillformat.SkillMainFile,
				Content: []byte("---\nname: github-review\ndescription: Review GitHub changes.\n---\nBody\n"),
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale file still exists, stat err: %v", err)
	}
	assertFileContains(t, filepath.Join(target, skillformat.SkillMainFile), "Review GitHub changes")
}

func TestClaudeCodeInstallSkillRejectsUnsafeArchivePaths(t *testing.T) {
	for _, relPath := range []string{"/tmp/escape", "../escape", "nested/../../escape"} {
		t.Run(relPath, func(t *testing.T) {
			_, err := NewClaudeCode().InstallSkill(context.Background(), t.TempDir(), SkillArchive{
				Name: "safe-name",
				Files: []SkillArchiveFile{
					{RelPath: skillformat.SkillMainFile, Content: []byte("---\nname: safe-name\ndescription: Safe.\n---\n")},
					{RelPath: relPath, Content: []byte("bad")},
				},
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.Contains(content, substr) {
		t.Fatalf("%s did not contain %q:\n%s", path, substr, content)
	}
}
