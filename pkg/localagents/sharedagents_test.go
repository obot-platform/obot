package localagents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/skillformat"
)

func TestSharedAgentsInstallBootstrapWritesExpectedSkills(t *testing.T) {
	home := t.TempDir()

	result, err := NewSharedAgents().InstallBootstrap(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}

	if result.AgentID != SharedAgentsID {
		t.Fatalf("AgentID = %q, want %q", result.AgentID, SharedAgentsID)
	}
	if len(result.Installed) != 4 {
		t.Fatalf("Installed count = %d, want 4: %#v", len(result.Installed), result.Installed)
	}

	for _, name := range []string{"obot", "obot-search-skills", "obot-install-skill", "obot-scan"} {
		content := readFile(t, filepath.Join(home, ".agents", "skills", name, skillformat.SkillMainFile))
		if !strings.Contains(content, "Obot") && !strings.Contains(content, "obot") {
			t.Fatalf("%s content did not look like an Obot bootstrap skill:\n%s", name, content)
		}
	}
}

func TestSharedAgentsInstallBootstrapOverwritesExistingContent(t *testing.T) {
	home := t.TempDir()
	oldSkill := filepath.Join(home, ".agents", "skills", "obot", skillformat.SkillMainFile)
	if err := os.MkdirAll(filepath.Dir(oldSkill), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldSkill, []byte("old local edit"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := NewSharedAgents().InstallBootstrap(t.Context(), home); err != nil {
		t.Fatal(err)
	}

	content := readFile(t, oldSkill)
	if strings.Contains(content, "old local edit") {
		t.Fatalf("bootstrap install preserved old content:\n%s", content)
	}
	if !strings.Contains(content, "rendered for `agents`") {
		t.Fatalf("bootstrap content was not replaced with rendered asset:\n%s", content)
	}
}

func TestSharedAgentsInstallSkillWritesSanitizedDirectory(t *testing.T) {
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

	result, err := NewSharedAgents().InstallSkill(t.Context(), home, skill)
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(home, ".agents", "skills", "github-review")
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
