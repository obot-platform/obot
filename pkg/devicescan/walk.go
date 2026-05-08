package devicescan

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/gobwas/glob"
)

// skillGlob is the pattern used to discover SKILL.md files during the
// project walk. Skill hits are routed separately from MCP hits.
const skillGlob = "**/SKILL.md"

// markerSkipDirs are basenames the project walk prunes when descending.
// The set covers dependency caches, build outputs, system / app-support
// trees that can't host project configs, and the macOS Trash. Matching
// is by basename, which loses some precision (the entire ~/Library tree
// is skipped, not just ~/Library/Caches) but is acceptable because
// client global configs are opened directly in Phase 1, not via the
// walk.
var markerSkipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".venv":        true,
	"venv":         true,
	"__pycache__":  true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".nuxt":        true,
	".turbo":       true,
	".cache":       true,
	".npm":         true,
	".yarn":        true,
	"Library":      true,
	"AppData":      true,
	".Trash":       true,
	"tmp":          true,
	"temp":         true,
}

// projectHit is one matched file from the walk paired with the scanner
// that owns it.
type projectHit struct {
	path    string
	scanner ClientScanner
}

// walkProject walks fsys from root once, matching every file against
// every scanner's ProjectGlobs and the SKILL.md pattern, returning two
// streams: scanner-attributed project hits, and skill-marker paths for
// scanProjectSkills.
//
// Depth is counted in path components: a top-level entry under root is
// depth 1, so maxDepth=N means "walk up to N path segments below the
// root" (top-level files matched at maxDepth=1).
//
// Files at fs-relative paths in skipPaths are dropped before
// dispatching (used to suppress the redundant project hit on a path
// that was already opened as a global config).
//
// Honours ctx: if cancelled / deadline-exceeded, the walk aborts early
// and returns whatever was matched so far.
func walkProject(ctx context.Context, fsys fs.FS, scanners []ClientScanner, maxDepth int, skipPaths map[string]bool) ([]projectHit, []string) {
	if fsys == nil {
		return nil, nil
	}

	type compiledScanner struct {
		scanner ClientScanner
		globs   []glob.Glob
	}
	var compiled []compiledScanner
	for _, sc := range scanners {
		patterns := sc.ProjectGlobs()
		if len(patterns) == 0 {
			continue
		}
		entry := compiledScanner{scanner: sc}
		for _, p := range patterns {
			g, err := glob.Compile(p, '/')
			if err != nil {
				log.Debugf("%s: skipping bad project glob %q: %v", sc.Name(), p, err)
				continue
			}
			entry.globs = append(entry.globs, g)
		}
		if len(entry.globs) > 0 {
			compiled = append(compiled, entry)
		}
	}
	skillPattern, _ := glob.Compile(skillGlob, '/')

	var (
		hits      []projectHit
		skillHits []string
	)
	_ = fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if markerSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			// depth=1 for top-level entries under root. SkipDir on a
			// dir at depth==maxDepth means we don't descend into it,
			// so files match at depths 1…maxDepth (inclusive).
			depth := strings.Count(rel, "/") + 1
			if depth >= maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if skipPaths[rel] {
			return nil
		}
		// Skill marker?
		if path.Base(rel) == "SKILL.md" && skillPattern != nil && skillPattern.Match(rel) {
			skillHits = append(skillHits, rel)
			return nil
		}
		// Scanner project glob match. First match wins (globs are
		// disjoint by design — Cursor uses .cursor/, VS Code uses
		// .vscode/, etc.).
		for _, c := range compiled {
			for _, g := range c.globs {
				if g.Match(rel) {
					hits = append(hits, projectHit{path: rel, scanner: c.scanner})
					return nil
				}
			}
		}
		return nil
	})
	return hits, skillHits
}
