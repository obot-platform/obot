package hostedagent

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/obot-platform/obot/pkg/agentbackend"
	gitpkg "github.com/obot-platform/obot/pkg/git"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// skillsMountRoot is where an agent finds its skills. Each skill gets a
	// directory named after the skill.
	skillsMountRoot = "/etc/obot/skills"

	// maxSkillFileBytes and maxSkillBytes bound what one skill can contribute.
	// Skill files land in a Kubernetes Secret, which is capped at 1MiB in
	// total, so a single oversized skill would otherwise make the whole sandbox
	// unschedulable with an error that says nothing about skills.
	maxSkillFileBytes = 256 << 10
	maxSkillBytes     = 512 << 10
)

// skillFetcher reads a skill's files out of the repository it was indexed from.
//
// Obot stores only the coordinates of a skill, never its content, so the files
// have to be fetched. Results are cached by commit: a skill pinned to a commit
// cannot change, and reconciliation is frequent enough that cloning every pass
// would be untenable.
type skillFetcher struct {
	mu    sync.Mutex
	cache map[string][]agentbackend.File

	// clone is injectable so tests do not need a git server.
	clone func(ctx context.Context, repoURL, token, ref string) (string, string, func(), error)
}

func newSkillFetcher() *skillFetcher {
	return &skillFetcher{
		cache: map[string][]agentbackend.File{},
		clone: gitpkg.Clone,
	}
}

// skillFiles returns the files for each skill and the config entries describing
// where they were placed.
//
// A skill that cannot be fetched is skipped rather than failing the sandbox: an
// agent usually has several, and one unreachable repository should not stop it
// from starting.
func (f *skillFetcher) skillFiles(ctx context.Context, client kclient.Client, namespace string, ids []string) ([]agentbackend.File, []agentConfigSkill) {
	var (
		files   []agentbackend.File
		entries []agentConfigSkill
		seen    = map[string]struct{}{}
	)

	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		var skill v1.Skill
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: id}, &skill); err != nil {
			log.Warnf("hosted agent: skipping skill %q: %v", id, err)
			continue
		}

		name := skillDirName(&skill)
		mount := path.Join(skillsMountRoot, name)

		skillFiles, err := f.fetch(ctx, &skill, mount)
		if err != nil {
			log.Warnf("hosted agent: skipping skill %q: %v", id, err)
			continue
		}

		files = append(files, skillFiles...)
		entries = append(entries, agentConfigSkill{
			ID:          skill.Name,
			Name:        name,
			Path:        mount,
			Description: skill.Spec.Description,
		})
	}
	return files, entries
}

// skillDirName is the directory a skill is installed under. The manifest name
// is preferred over the generated ID because an agent reads these paths.
func skillDirName(skill *v1.Skill) string {
	name := strings.TrimSpace(skill.Spec.Name)
	if name == "" {
		name = skill.Name
	}
	return sanitizeSkillName(name)
}

// sanitizeSkillName keeps a skill name usable as a single path segment. A name
// comes from a third-party repository, so "../" must not be able to place files
// outside the skills directory.
func sanitizeSkillName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), ".-")
	if result == "" {
		return "skill"
	}
	return result
}

func (f *skillFetcher) fetch(ctx context.Context, skill *v1.Skill, mount string) ([]agentbackend.File, error) {
	if skill.Spec.RepoURL == "" {
		return nil, fmt.Errorf("skill has no repository")
	}

	key := skill.Spec.RepoURL + "@" + skill.Spec.CommitSHA + "#" + skill.Spec.RelativePath + "->" + mount
	// Only a commit-pinned skill is cacheable. Without a commit the repository
	// could have moved, and serving a stale checkout would silently pin the
	// agent to an old version of the skill.
	cacheable := skill.Spec.CommitSHA != ""

	if cacheable {
		f.mu.Lock()
		cached, ok := f.cache[key]
		f.mu.Unlock()
		if ok {
			return cached, nil
		}
	}

	ref := skill.Spec.CommitSHA
	if ref == "" {
		ref = skill.Spec.RepoRef
	}
	dir, _, cleanup, err := f.clone(ctx, skill.Spec.RepoURL, "", ref)
	if err != nil {
		return nil, fmt.Errorf("clone %s: %w", skill.Spec.RepoURL, err)
	}
	defer cleanup()

	root := dir
	if skill.Spec.RelativePath != "" {
		root = filepath.Join(dir, filepath.FromSlash(skill.Spec.RelativePath))
	}
	// The skill's own directory is what gets installed, not the file naming it.
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		root = filepath.Dir(root)
	}

	files, err := readSkillDir(root, mount)
	if err != nil {
		return nil, err
	}

	if cacheable {
		f.mu.Lock()
		f.cache[key] = files
		f.mu.Unlock()
	}
	return files, nil
}

// readSkillDir collects a skill's files, rooted at the mount point it will be
// installed under.
func readSkillDir(root, mount string) ([]agentbackend.File, error) {
	var (
		files []agentbackend.File
		total int
	)

	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// A symlink could point anywhere in the checkout, or out of it.
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSkillFileBytes {
			return fmt.Errorf("%s is larger than the %d byte per-file limit", entry.Name(), maxSkillFileBytes)
		}
		total += int(info.Size())
		if total > maxSkillBytes {
			return fmt.Errorf("skill exceeds the %d byte limit", maxSkillBytes)
		}

		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		files = append(files, agentbackend.File{
			Path:    path.Join(mount, filepath.ToSlash(rel)),
			Content: content,
			Mode:    0o444,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Stable order: the file set feeds the desired revision, and a walk that
	// reordered itself would restart every sandbox for no reason.
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
