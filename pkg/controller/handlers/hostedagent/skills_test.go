package hostedagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentbackend"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeRepo writes a skill directory and returns a clone function serving it.
func fakeRepo(t *testing.T, files map[string]string) (func(context.Context, string, string, string) (string, string, func(), error), *int) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	return func(context.Context, string, string, string) (string, string, func(), error) {
		calls++
		return dir, "sha", func() {}, nil
	}, &calls
}

func skillClient(skills ...*v1.Skill) *fake.ClientBuilder {
	builder := fake.NewClientBuilder().WithScheme(storagescheme.Scheme)
	for _, skill := range skills {
		builder = builder.WithObjects(skill)
	}
	return builder
}

// Obot stores only a skill's coordinates, so the files have to be fetched and
// placed in the sandbox. An agent should find its skills on disk rather than
// being told where to go and get them.
func TestSkillFilesArePlacedInTheSandbox(t *testing.T) {
	clone, calls := fakeRepo(t, map[string]string{
		"SKILL.md":           "# PDF skill",
		"scripts/extract.py": "print('hi')",
		".git/config":        "should not be copied",
	})
	fetcher := &skillFetcher{cache: map[string][]agentbackend.File{}, clone: clone}

	client := skillClient(&v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1pdf", Namespace: "obot"},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{Name: "pdf", Description: "Work with PDFs"},
			RepoURL:       "https://example.com/skills.git",
			CommitSHA:     "abc123",
		},
	}).Build()

	files, entries := fetcher.skillFiles(context.Background(), client, "obot", []string{"sk1pdf"})

	if len(entries) != 1 || entries[0].Path != "/etc/obot/skills/pdf" {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Description != "Work with PDFs" {
		t.Errorf("description = %q", entries[0].Description)
	}

	paths := map[string]string{}
	for _, file := range files {
		paths[file.Path] = string(file.Content)
	}
	if paths["/etc/obot/skills/pdf/SKILL.md"] != "# PDF skill" {
		t.Errorf("SKILL.md missing or wrong: %v", paths)
	}
	if paths["/etc/obot/skills/pdf/scripts/extract.py"] == "" {
		t.Errorf("nested file missing: %v", paths)
	}
	for path := range paths {
		if strings.Contains(path, ".git") {
			t.Errorf("git metadata was copied: %s", path)
		}
	}

	// A pinned skill cannot change, so it is fetched once however often the
	// instance reconciles.
	fetcher.skillFiles(context.Background(), client, "obot", []string{"sk1pdf"})
	if *calls != 1 {
		t.Errorf("clone calls = %d, want 1 (the result should be cached)", *calls)
	}
}

// A skill name comes from a third-party repository, so it must not be able to
// place files outside the skills directory.
func TestSkillNameCannotEscapeTheSkillsDirectory(t *testing.T) {
	for _, tt := range []struct{ name, want string }{
		{"pdf", "pdf"},
		{"../../etc/passwd", "etc-passwd"},
		{"a/b", "a-b"},
		{"..", "skill"},
		// With no manifest name the object name is used, which is still a
		// stable, unique directory.
		{"", "sk1"},
	} {
		skill := &v1.Skill{
			ObjectMeta: metav1.ObjectMeta{Name: "sk1"},
			Spec:       v1.SkillSpec{SkillManifest: types.SkillManifest{Name: tt.name}},
		}
		if got := skillDirName(skill); got != tt.want {
			t.Errorf("skillDirName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// One unreachable skill repository should not stop an agent that has others.
func TestUnfetchableSkillIsSkipped(t *testing.T) {
	fetcher := &skillFetcher{cache: map[string][]agentbackend.File{}}
	client := skillClient(&v1.Skill{
		ObjectMeta: metav1.ObjectMeta{Name: "sk1broken", Namespace: "obot"},
		Spec:       v1.SkillSpec{SkillManifest: types.SkillManifest{Name: "broken"}},
	}).Build()

	files, entries := fetcher.skillFiles(context.Background(), client, "obot", []string{"sk1broken", "sk1missing"})
	if len(files) != 0 || len(entries) != 0 {
		t.Fatalf("expected nothing for unfetchable skills: %v %v", files, entries)
	}
}
