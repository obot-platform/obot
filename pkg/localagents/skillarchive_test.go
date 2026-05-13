package localagents

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/skillformat"
)

func TestParseSkillArchiveValidRootZip(t *testing.T) {
	archive, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{
		{
			Name:    skillformat.SkillMainFile,
			Content: "---\nname: frontmatter-name\ndescription: Test skill.\n---\nBody\n",
			Mode:    0644,
		},
		{
			Name:    "scripts/run.sh",
			Content: "#!/bin/sh\nexit 0\n",
			Mode:    0755,
		},
	}), "fallback-name")
	if err != nil {
		t.Fatal(err)
	}

	if archive.Name != "frontmatter-name" {
		t.Fatalf("Name = %q, want frontmatter-name", archive.Name)
	}
	if len(archive.Files) != 2 {
		t.Fatalf("Files count = %d, want 2", len(archive.Files))
	}

	name, err := archive.installName()
	if err != nil {
		t.Fatal(err)
	}
	if name != "frontmatter-name" {
		t.Fatalf("installName = %q, want frontmatter-name", name)
	}
}

func TestParseSkillArchiveStripsSingleRootDirectory(t *testing.T) {
	archive, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{
		{
			Name:    "wrapped/SKILL.md",
			Content: "---\nname: wrapped-skill\ndescription: Wrapped skill.\n---\n",
			Mode:    0644,
		},
		{
			Name:    "wrapped/docs/readme.md",
			Content: "# Read me\n",
			Mode:    0644,
		},
	}), "fallback-name")
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, file := range archive.Files {
		got[file.RelPath] = true
	}
	for _, rel := range []string{"SKILL.md", "docs/readme.md"} {
		if !got[rel] {
			t.Fatalf("expected rooted file %q in %#v", rel, got)
		}
	}
}

func TestParseSkillArchiveRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name  string
		entry zipTestEntry
	}{
		{
			name:  "absolute path",
			entry: zipTestEntry{Name: "/tmp/escape", Content: "bad"},
		},
		{
			name:  "windows absolute path",
			entry: zipTestEntry{Name: `C:\tmp\escape`, Content: "bad"},
		},
		{
			name:  "parent traversal",
			entry: zipTestEntry{Name: "../escape", Content: "bad"},
		},
		{
			name:  "internal parent traversal",
			entry: zipTestEntry{Name: "safe/../escape", Content: "bad"},
		},
		{
			name:  "symlink",
			entry: zipTestEntry{Name: "link", Content: "target", Mode: os.ModeSymlink | 0777},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{
				{
					Name:    skillformat.SkillMainFile,
					Content: "---\nname: safe-name\ndescription: Safe.\n---\n",
					Mode:    0644,
				},
				tt.entry,
			}), "safe-name")
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseSkillArchiveRequiresSkillMD(t *testing.T) {
	_, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{{
		Name:    "README.md",
		Content: "# Missing skill\n",
		Mode:    0644,
	}}), "fallback-name")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), skillformat.SkillMainFile) {
		t.Fatalf("error = %v, want mention %s", err, skillformat.SkillMainFile)
	}
}

func TestParseSkillArchiveFallsBackToSanitizedName(t *testing.T) {
	archive, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{{
		Name:    skillformat.SkillMainFile,
		Content: "# No frontmatter\n",
		Mode:    0644,
	}}), "GitHub Review!")
	if err != nil {
		t.Fatal(err)
	}

	name, err := archive.installName()
	if err != nil {
		t.Fatal(err)
	}
	if name != "github-review" {
		t.Fatalf("installName = %q, want github-review", name)
	}
}

func TestSkillArchiveExtractToWritesConservativeModes(t *testing.T) {
	target := filepath.Join(t.TempDir(), "skill")
	archive, err := ParseSkillArchive(buildSkillZip(t, []zipTestEntry{
		{
			Name:    skillformat.SkillMainFile,
			Content: "---\nname: extract-me\ndescription: Extract me.\n---\n",
			Mode:    0666,
		},
		{
			Name:    "scripts/run.sh",
			Content: "#!/bin/sh\n",
			Mode:    0777,
		},
	}), "fallback-name")
	if err != nil {
		t.Fatal(err)
	}

	if err := archive.ExtractTo(target); err != nil {
		t.Fatal(err)
	}

	assertFileContains(t, filepath.Join(target, skillformat.SkillMainFile), "extract-me")
	assertFileContains(t, filepath.Join(target, "scripts", "run.sh"), "#!/bin/sh")

	mainInfo, err := os.Stat(filepath.Join(target, skillformat.SkillMainFile))
	if err != nil {
		t.Fatal(err)
	}
	if mainInfo.Mode().Perm() != 0644 {
		t.Fatalf("SKILL.md mode = %v, want 0644", mainInfo.Mode().Perm())
	}

	scriptInfo, err := os.Stat(filepath.Join(target, "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if scriptInfo.Mode().Perm() != 0755 {
		t.Fatalf("script mode = %v, want 0755", scriptInfo.Mode().Perm())
	}
}

type zipTestEntry struct {
	Name    string
	Content string
	Mode    os.FileMode
	Dir     bool
}

func buildSkillZip(t *testing.T, entries []zipTestEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.Name}
		if entry.Dir {
			header.Name = strings.TrimSuffix(header.Name, "/") + "/"
			header.SetMode(0755 | os.ModeDir)
		} else if entry.Mode != 0 {
			header.SetMode(entry.Mode)
		} else {
			header.SetMode(0644)
		}
		fileWriter, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if !entry.Dir {
			if _, err := fileWriter.Write([]byte(entry.Content)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
