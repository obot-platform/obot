package skillrepository

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createSkillDir creates a directory with a SKILL.md file containing valid frontmatter.
func createSkillDir(t *testing.T, parent, dirName, name, description string) string {
	t.Helper()
	dir := filepath.Join(parent, dirName)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n# %s\nBody.\n", name, description, name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
	return dir
}

func TestDiscoverSkillDirectories(t *testing.T) {
	t.Run("single skill at root", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# skill"), 0o644))

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		require.Len(t, dirs, 1)
		assert.Equal(t, root, dirs[0])
	})

	t.Run("multiple skills sorted", func(t *testing.T) {
		root := t.TempDir()
		createSkillDir(t, root, "skill-b", "skill-b", "B")
		createSkillDir(t, root, "skill-a", "skill-a", "A")

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		require.Len(t, dirs, 2)
		assert.True(t, strings.HasSuffix(dirs[0], "skill-a"))
		assert.True(t, strings.HasSuffix(dirs[1], "skill-b"))
	})

	t.Run("nested skills", func(t *testing.T) {
		root := t.TempDir()
		createSkillDir(t, filepath.Join(root, "category"), "my-skill", "my-skill", "nested")

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		require.Len(t, dirs, 1)
	})

	t.Run("no skills", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("# readme"), 0o644))

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		assert.Empty(t, dirs)
	})

	t.Run("SKILL.md is a directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, "SKILL.md"), 0o755))

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		assert.Empty(t, dirs)
	})

	t.Run("symlink directory skipped", func(t *testing.T) {
		root := t.TempDir()
		realDir := createSkillDir(t, root, "real-skill", "real-skill", "real")
		linkDir := filepath.Join(root, "linked-skill")
		if err := os.Symlink(realDir, linkDir); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		// Only the real directory should be discovered
		require.Len(t, dirs, 1)
		assert.Equal(t, realDir, dirs[0])
	})

	t.Run("symlink SKILL.md rejected", func(t *testing.T) {
		root := t.TempDir()
		skillDir := filepath.Join(root, "my-skill")
		require.NoError(t, os.MkdirAll(skillDir, 0o755))

		// Create a real SKILL.md somewhere else and symlink to it
		realFile := filepath.Join(root, "real-skill.md")
		require.NoError(t, os.WriteFile(realFile, []byte("# real"), 0o644))
		if err := os.Symlink(realFile, filepath.Join(skillDir, "SKILL.md")); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		// discoverSkillDirectories uses os.Lstat, which inspects the symlink
		// itself rather than following it. The ModeSymlink bit will be set,
		// so the function rejects it as expected.
		_, err := discoverSkillDirectories(root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symbolic links are not allowed")
	})

	t.Run("skill at root and subdirectory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("# root"), 0o644))
		createSkillDir(t, root, "sub-skill", "sub-skill", "sub")

		dirs, err := discoverSkillDirectories(root)
		require.NoError(t, err)
		require.Len(t, dirs, 2)
	})
}

func testRepo(name, namespace string) *v1.SkillRepository {
	return &v1.SkillRepository{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "SkillRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.SkillRepositorySpec{
			SkillRepositoryManifest: types.SkillRepositoryManifest{
				RepoURL: "https://github.com/owner/repo",
				Ref:     "main",
			},
		},
	}
}

func TestBuildSkill(t *testing.T) {
	repo := testRepo("repo1", "default")
	commitSHA := "abc123"
	indexedAt := metav1.Now()

	t.Run("valid frontmatter name matches dir", func(t *testing.T) {
		parent := t.TempDir()
		dir := createSkillDir(t, parent, "my-skill", "my-skill", "A test skill")

		skill, err := buildSkill(dir, "my-skill", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.True(t, skill.Status.Valid)
		assert.Empty(t, skill.Status.ValidationError)
		assert.Equal(t, "my-skill", skill.Spec.Name)
		assert.Equal(t, "A test skill", skill.Spec.Description)
		assert.Equal(t, "My Skill", skill.Spec.DisplayName)
		assert.Equal(t, "repo1", skill.Spec.RepoID)
		assert.Equal(t, "https://github.com/owner/repo", skill.Spec.RepoURL)
		assert.Equal(t, "main", skill.Spec.RepoRef)
		assert.Equal(t, commitSHA, skill.Spec.CommitSHA)
		assert.Equal(t, "my-skill", skill.Spec.RelativePath)
		assert.NotEmpty(t, skill.Spec.InstallHash)
		assert.Equal(t, "default", skill.Namespace)
	})

	t.Run("missing name uses dir basename", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "fallback-dir")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := "---\ndescription: no name field\n---\n# Body\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		skill, err := buildSkill(dir, "fallback-dir", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		// Name falls back to dir basename, but validation will fail because
		// ValidateFrontmatter checks name is not empty
		assert.Equal(t, "fallback-dir", skill.Spec.Name)
		assert.False(t, skill.Status.Valid)
	})

	t.Run("invalid name uppercase", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "BadName")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := "---\nname: BadName\ndescription: bad\n---\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		skill, err := buildSkill(dir, "BadName", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.False(t, skill.Status.Valid)
		assert.NotEmpty(t, skill.Status.ValidationError)
	})

	t.Run("name does not match dir", func(t *testing.T) {
		parent := t.TempDir()
		dir := createSkillDir(t, parent, "actual-dir", "different-name", "mismatched")

		skill, err := buildSkill(dir, "actual-dir", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.False(t, skill.Status.Valid)
		assert.Contains(t, skill.Status.ValidationError, "does not match")
	})

	t.Run("no frontmatter", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "no-fm")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Just markdown"), 0o644))

		skill, err := buildSkill(dir, "no-fm", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.Equal(t, "no-fm", skill.Spec.Name) // falls back to dir name
		assert.False(t, skill.Status.Valid)
	})

	t.Run("with metadata and license", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "full-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := `---
name: full-skill
description: A fully specified skill
license: MIT
compatibility: "nanobot >= 1.0"
metadata:
  author: tester
  version: "1.0"
allowed-tools: tool1,tool2
---
# Full Skill
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))

		skill, err := buildSkill(dir, "full-skill", repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.True(t, skill.Status.Valid)
		assert.Equal(t, "MIT", skill.Spec.License)
		assert.Equal(t, "nanobot >= 1.0", skill.Spec.Compatibility)
		assert.Equal(t, "tool1,tool2", skill.Spec.AllowedTools)
		assert.Equal(t, "tester", skill.Spec.MetadataValues["author"])
		assert.Equal(t, "1.0", skill.Spec.MetadataValues["version"])
	})

	t.Run("SKILL.md exceeds size limit", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "big-skill")
		require.NoError(t, os.MkdirAll(dir, 0o755))
		bigContent := strings.Repeat("x", maxSkillMDBytes+1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(bigContent), 0o644))

		_, err := buildSkill(dir, "big-skill", repo, commitSHA, indexedAt)
		require.Error(t, err) // hard error, not validation
		assert.Contains(t, err.Error(), "exceeds maximum size")
	})
}

func TestBuildSkillsFromRepository(t *testing.T) {
	repo := testRepo("repo1", "default")
	commitSHA := "abc123"
	indexedAt := metav1.Now()

	t.Run("two valid skills", func(t *testing.T) {
		root := t.TempDir()
		createSkillDir(t, root, "skill-a", "skill-a", "Skill A")
		createSkillDir(t, root, "skill-b", "skill-b", "Skill B")

		skills, err := buildSkillsFromRepository(root, repo, commitSHA, indexedAt)
		require.NoError(t, err)
		require.Len(t, skills, 2)

		assert.Equal(t, "skill-a", skills[0].Spec.RelativePath)
		assert.Equal(t, "skill-b", skills[1].Spec.RelativePath)
		for _, s := range skills {
			assert.Equal(t, "repo1", s.Spec.RepoID)
			assert.Equal(t, "default", s.Namespace)
			assert.Equal(t, commitSHA, s.Spec.CommitSHA)
		}
	})

	t.Run("valid and invalid skill", func(t *testing.T) {
		root := t.TempDir()
		createSkillDir(t, root, "good-skill", "good-skill", "Good")

		// Create an invalid skill (name doesn't match dir)
		badDir := filepath.Join(root, "bad-dir")
		require.NoError(t, os.MkdirAll(badDir, 0o755))
		content := "---\nname: wrong-name\ndescription: mismatch\n---\n"
		require.NoError(t, os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte(content), 0o644))

		skills, err := buildSkillsFromRepository(root, repo, commitSHA, indexedAt)
		require.NoError(t, err)
		require.Len(t, skills, 2)

		var validCount, invalidCount int
		for _, s := range skills {
			if s.Status.Valid {
				validCount++
			} else {
				invalidCount++
			}
		}
		assert.Equal(t, 1, validCount)
		assert.Equal(t, 1, invalidCount)
	})

	t.Run("empty repo", func(t *testing.T) {
		root := t.TempDir()

		skills, err := buildSkillsFromRepository(root, repo, commitSHA, indexedAt)
		require.NoError(t, err)
		assert.Empty(t, skills)
	})
}

func TestComputeInstallHash(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644))

		hash1, err := computeInstallHash(dir)
		require.NoError(t, err)
		hash2, err := computeInstallHash(dir)
		require.NoError(t, err)
		assert.Equal(t, hash1, hash2)
	})

	t.Run("content change produces different hash", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))

		hash1, err := computeInstallHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644))
		hash2, err := computeInstallHash(dir)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("filename change produces different hash", func(t *testing.T) {
		dir1 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir1, "file-a.txt"), []byte("content"), 0o644))
		hash1, err := computeInstallHash(dir1)
		require.NoError(t, err)

		dir2 := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir2, "file-b.txt"), []byte("content"), 0o644))
		hash2, err := computeInstallHash(dir2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("added file changes hash", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))

		hash1, err := computeInstallHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("extra"), 0o644))
		hash2, err := computeInstallHash(dir)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		hash, err := computeInstallHash(dir)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
	})

	t.Run("symlink rejected", func(t *testing.T) {
		dir := t.TempDir()
		realFile := filepath.Join(dir, "real.txt")
		require.NoError(t, os.WriteFile(realFile, []byte("real"), 0o644))
		if err := os.Symlink(realFile, filepath.Join(dir, "link.txt")); err != nil {
			t.Skip("symlinks not supported on this platform")
		}

		_, err := computeInstallHash(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "symbolic links")
	})

	t.Run("subdirectories included", func(t *testing.T) {
		dir := t.TempDir()
		subDir := filepath.Join(dir, "sub")
		require.NoError(t, os.MkdirAll(subDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0o644))

		hash, err := computeInstallHash(dir)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
	})
}

func TestSkillObjectName(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		name := skillObjectName("repo1", "my-skill")
		assert.Contains(t, name, "sk1")
		assert.NotEmpty(t, name)
	})

	t.Run("nested path", func(t *testing.T) {
		name := skillObjectName("repo1", "category/my-skill")
		assert.Contains(t, name, "sk1")
	})

	t.Run("empty relPath uses skill", func(t *testing.T) {
		name := skillObjectName("repo1", "")
		assert.Contains(t, name, "sk1")
		assert.Contains(t, name, "skill")
	})

	t.Run("deterministic", func(t *testing.T) {
		name1 := skillObjectName("repo1", "my-skill")
		name2 := skillObjectName("repo1", "my-skill")
		assert.Equal(t, name1, name2)
	})

	t.Run("different inputs differ", func(t *testing.T) {
		name1 := skillObjectName("repo1", "skill-a")
		name2 := skillObjectName("repo1", "skill-b")
		assert.NotEqual(t, name1, name2)
	})
}

func TestSanitizeNameFragment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "simple lowercase", input: "my-skill", expected: "my-skill"},
		{name: "slashes to dashes", input: "a/b/c", expected: "a-b-c"},
		{name: "underscores to dashes", input: "a_b", expected: "a-b"},
		{name: "dots to dashes", input: "v1.0", expected: "v1-0"},
		{name: "spaces to dashes", input: "a b", expected: "a-b"},
		{name: "uppercase lowered", input: "UPPER", expected: "upper"},
		{name: "consecutive specials collapsed", input: "a//b", expected: "a-b"},
		{name: "leading trailing stripped", input: "/path/", expected: "path"},
		{name: "empty string", input: "", expected: ""},
		{name: "mixed special chars", input: "My_Skill.v2/Sub Dir", expected: "my-skill-v2-sub-dir"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeNameFragment(tt.input))
		})
	}
}
