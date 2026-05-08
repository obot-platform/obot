package devicescan

import (
	"io/fs"
	"path"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

// multiClient is the synthetic client tag for SKILL.md files that we
// can't pin to a specific client (e.g. `.agents/skills/...`,
// free-floating project skills). It appears on observation rows so
// consumers can group them, but the orchestrator's build step suppresses
// it from the top-level clients[] dimension because no real client owns
// these.
const multiClient = "multi"

var (
	// skillExts is the extension allowlist for files counted as part of
	// a skill's manifest. The paths are listed on Skill.Files but only
	// SKILL.md content is uploaded into the scan's top-level files[] —
	// the rest are path-only references.
	skillExts = map[string]bool{
		".md":  true,
		".mdc": true,
		".txt": true,
		".sh":  true,
		".py":  true,
		".js":  true,
		".ts":  true,
	}

	// globalSkillDirs are home-relative directories whose immediate
	// children are skill directories. The tool tag is wired into Client
	// on the wire observation. Dirs with no canonical owning client
	// (`.agents/skills`, `.agent/skills`) are intentionally absent —
	// skills found in those locations come through scanProjectSkills
	// with client=multiClient ("multi") instead.
	globalSkillDirs = []struct {
		rel  string
		tool string
	}{
		{".claude/skills", "claude_code"},
		{".codex/skills", "codex"},
		{".config/opencode/skills", "opencode"},
		{".skillport/skills", "skillport"},
	}

	// homeClientTool maps the first home-relative path component to a
	// tool tag, used to attribute SKILL.md files found anywhere under
	// that component to the right client with scope=user.
	homeClientTool = map[string]string{
		".cursor":    "cursor",
		".claude":    "claude_code",
		".codex":     "codex",
		".codeium":   "windsurf",
		".windsurf":  "windsurf",
		".hermes":    "hermes",
		".skillport": "skillport",
	}
)

// scanGlobalSkills walks each globalSkillDirs entry and emits one skill
// per immediate-child directory containing a SKILL.md.
func scanGlobalSkills(s *scanState) []types.DeviceScanSkill {
	var out []types.DeviceScanSkill
	for _, gd := range globalSkillDirs {
		entries, err := fs.ReadDir(s.fsys, gd.rel)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			skillDir := path.Join(gd.rel, e.Name())
			if !fileExists(s.fsys, path.Join(skillDir, "SKILL.md")) {
				continue
			}
			if sk, ok := ingestSkill(s, skillDir, gd.tool, ""); ok {
				out = append(out, sk)
			}
		}
	}
	return out
}

// scanProjectSkills consumes the skill-marker hits from the project
// walk and emits one skill per directory. SKILL.md files inside known
// global skill prefixes are skipped (scanGlobalSkills handles them).
// Hits under a known home dot-dir are attributed to that client; the
// rest are attributed to multiClient.
func scanProjectSkills(s *scanState, skillMarkers []string) []types.DeviceScanSkill {
	prefixes := globalSkillPrefixes()
	seen := map[string]bool{}
	var out []types.DeviceScanSkill
	for _, m := range skillMarkers {
		if hasAnyPrefix(m, prefixes) {
			continue
		}
		skillDir := path.Dir(m)
		if seen[skillDir] {
			continue
		}
		seen[skillDir] = true

		if tool, ok := inferHomeTool(m); ok {
			if sk, ok2 := ingestSkill(s, skillDir, tool, ""); ok2 {
				out = append(out, sk)
			}
		} else {
			if sk, ok := ingestSkill(s, skillDir, multiClient, s.abs(skillDir)); ok {
				out = append(out, sk)
			}
		}
	}
	return out
}

// ingestSkill builds a DeviceScanSkill for the directory at
// skillDirRel. client may be "" for free-floating SKILL.md files with
// no client owner. projectPathAbs is the absolute project root for
// project-scope skills, "" otherwise.
func ingestSkill(s *scanState, skillDirRel, client, projectPathAbs string) (types.DeviceScanSkill, bool) {
	markerRel := path.Join(skillDirRel, "SKILL.md")
	markerData, err := fs.ReadFile(s.fsys, markerRel)
	if err != nil {
		return types.DeviceScanSkill{}, false
	}
	name, description := parseFrontmatter(markerData)
	if name == "" {
		name = clipRunes(path.Base(skillDirRel), skillNameMaxRunes)
	}

	markerAbs := s.addFileOrAbs(markerRel)
	hasScripts := dirExists(s.fsys, path.Join(skillDirRel, "scripts"))
	gitURL := readGitOrigin(s.fsys, skillDirRel)

	files := s.listArtifactPaths(skillDirRel, skillExts)

	return types.DeviceScanSkill{
		Client:       client,
		ProjectPath:  projectPathAbs,
		File:         markerAbs,
		Name:         name,
		Description:  description,
		Files:        files,
		HasScripts:   hasScripts,
		GitRemoteURL: gitURL,
	}, true
}

// globalSkillPrefixes returns the set of fs-relative path prefixes that
// scanGlobalSkills owns; SKILL.md files under these prefixes are
// skipped by scanProjectSkills to avoid double-counting.
func globalSkillPrefixes() []string {
	out := make([]string, 0, len(globalSkillDirs))
	for _, gd := range globalSkillDirs {
		out = append(out, gd.rel+"/")
	}
	return out
}

func hasAnyPrefix(rel string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

// inferHomeTool returns the tool tag if rel is under a known home
// dot-directory (e.g. .claude/.../SKILL.md → claude_code).
func inferHomeTool(rel string) (string, bool) {
	first, _, _ := strings.Cut(rel, "/")
	tool, ok := homeClientTool[first]
	return tool, ok
}
