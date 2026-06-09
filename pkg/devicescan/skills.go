package devicescan

import (
	"io/fs"
	"path"
	"slices"
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

	// Cursor documents reading .cursor/skills and .agents/skills (home
	// and project, anywhere in a repo) plus .claude/skills and
	// .codex/skills for compatibility: https://cursor.com/docs/skills
	globalSkillDirs = []skillRootAttribution{
		{rel: ".agents/skills", clients: []string{multiClient}, compatibleClients: []string{"cursor", "vscode"}},
		{rel: ".claude/skills", clients: []string{"claude_code"}, compatibleClients: []string{"cursor", "vscode"}},
		{rel: ".codex/skills", clients: []string{"codex"}, compatibleClients: []string{"cursor"}},
		{rel: ".config/opencode/skills", clients: []string{"opencode"}},
		{rel: ".copilot/skills", clients: []string{"vscode"}},
		{rel: ".cursor/skills", clients: []string{"cursor"}},
		{rel: ".skillport/skills", clients: []string{"skillport"}},
	}

	projectSkillRoots = []skillRootAttribution{
		{rel: ".agents/skills", clients: []string{multiClient}, compatibleClients: []string{"cursor", "vscode"}},
		{rel: ".claude/skills", clients: []string{"claude_code"}, compatibleClients: []string{"cursor", "vscode"}},
		{rel: ".codex/skills", clients: []string{"codex"}, compatibleClients: []string{"cursor"}},
		{rel: ".cursor/skills", clients: []string{"cursor"}},
		{rel: ".github/skills", clients: []string{multiClient}, compatibleClients: []string{"vscode"}},
	}

	homeClientSkillRoots = []skillRootAttribution{
		{rel: ".cursor", clients: []string{"cursor"}},
		{rel: ".claude", clients: []string{"claude_code"}},
		{rel: ".codex", clients: []string{"codex"}},
		{rel: ".codeium", clients: []string{"windsurf"}},
		{rel: ".windsurf", clients: []string{"windsurf"}},
		{rel: ".hermes", clients: []string{"hermes"}},
		{rel: ".skillport", clients: []string{"skillport"}},
	}
)

// skillRootAttribution maps a skill root directory to the clients its
// skills are attributed to. clients are the canonical owners and are
// always attributed; compatibleClients are other clients known to read
// the location and are attributed only when presence detection found
// them installed on the device.
type skillRootAttribution struct {
	rel               string
	clients           []string
	compatibleClients []string
}

// attributionsFor returns the full attribution set for skills under
// this root: the unconditional owners plus any compatible client that
// is presence-detected on the device.
func (r skillRootAttribution) attributionsFor(s *scanState) []string {
	out := make([]string, 0, len(r.clients)+len(r.compatibleClients))
	out = append(out, r.clients...)
	for _, client := range r.compatibleClients {
		if s.clientDetected(client) {
			out = append(out, client)
		}
	}
	return out
}

// scanGlobalSkills walks each globalSkillDirs entry recursively and
// emits one skill per directory containing a SKILL.md. Recursion
// matches client behavior — Cursor walks skills roots recursively, so
// skills may be organized in category subdirectories. A matched skill
// directory's subtree is not searched further, so SKILL.md files
// nested inside a skill (e.g. bundled examples) aren't counted as
// separate skills.
func scanGlobalSkills(s *scanState) []types.DeviceScanSkill {
	var out []types.DeviceScanSkill
	for _, gd := range globalSkillDirs {
		clients := gd.attributionsFor(s)
		_ = fs.WalkDir(s.fsys, gd.rel, func(rel string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !d.IsDir() || rel == gd.rel {
				return nil
			}
			if artifactSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			if !fileExists(s.fsys, path.Join(rel, "SKILL.md")) {
				return nil
			}
			out = append(out, ingestSkillAttributions(s, rel, clients, "")...)
			return fs.SkipDir
		})
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

		clients, projectPathAbs := inferSkillAttributions(s, skillDir)
		out = append(out, ingestSkillAttributions(s, skillDir, clients, projectPathAbs)...)
	}
	return out
}

func ingestSkillAttributions(s *scanState, skillDirRel string, clients []string, projectPathAbs string) []types.DeviceScanSkill {
	clients = uniqueNonEmpty(clients)
	if len(clients) == 0 {
		return nil
	}
	var out []types.DeviceScanSkill
	for _, client := range clients {
		if sk, ok := ingestSkill(s, skillDirRel, client, projectPathAbs); ok {
			sk.Clients = append([]string(nil), clients...)
			out = append(out, sk)
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
		Clients:      []string{client},
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

func inferSkillAttributions(s *scanState, skillDirRel string) ([]string, string) {
	if root, ok := rootForSkill(skillDirRel, projectSkillRoots); ok {
		return root.attributionsFor(s), projectRootForSkill(s, skillDirRel)
	}
	if root, ok := rootForSkill(skillDirRel, homeClientSkillRoots); ok {
		return root.attributionsFor(s), homeClientProjectPath(s, skillDirRel, root)
	}
	return []string{multiClient}, s.abs(skillDirRel)
}

// homeClientProjectPath returns "" (user scope) for skills directly
// under a home client dot-dir, and the containing directory when the
// client dot-dir is nested inside a project tree (e.g.
// work/repo/.windsurf/skills/x → work/repo) so project scope isn't
// lost just because the location pins a client.
func homeClientProjectPath(s *scanState, skillDirRel string, root skillRootAttribution) string {
	if skillDirRel == root.rel || strings.HasPrefix(skillDirRel, root.rel+"/") {
		return ""
	}
	if idx := strings.Index(skillDirRel, "/"+root.rel+"/"); idx > 0 {
		return s.abs(skillDirRel[:idx])
	}
	return ""
}

func rootForSkill(rel string, roots []skillRootAttribution) (skillRootAttribution, bool) {
	for _, root := range roots {
		if rel == root.rel || strings.HasPrefix(rel, root.rel+"/") || strings.Contains(rel, "/"+root.rel+"/") {
			return root, true
		}
	}
	return skillRootAttribution{}, false
}

func projectRootForSkill(s *scanState, skillDirRel string) string {
	for _, root := range projectSkillRoots {
		needle := "/" + root.rel + "/"
		if idx := strings.Index(skillDirRel, needle); idx > 0 {
			return s.abs(skillDirRel[:idx])
		}
		if skillDirRel == root.rel || strings.HasPrefix(skillDirRel, root.rel+"/") {
			// The skill root sits at the top of the walk, so the
			// project root is the walk root (the home directory).
			return s.abs(".")
		}
	}
	return s.abs(skillDirRel)
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
