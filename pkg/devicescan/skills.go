package devicescan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"gopkg.in/yaml.v3"
)

// skillExts is the extension allowlist for files counted as part of a
// skill's manifest. Path-only (the bytes are not uploaded; only
// SKILL.md content goes into files[]).
var skillExts = map[string]struct{}{
	".md":  {},
	".mdc": {},
	".txt": {},
	".sh":  {},
	".py":  {},
	".js":  {},
	".ts":  {},
}

// homeDirClients maps the first home-relative path component to the
// client tag used to attribute SKILL.md files found anywhere under
// that component back to the right client.
var homeDirClients = map[string]string{
	".cursor":    "cursor",
	".claude":    "claude_code",
	".codex":     "codex",
	".codeium":   "windsurf",
	".windsurf":  "windsurf",
	".hermes":    "hermes",
	".skillport": "skillport",
}

// projectSkills is the synthetic client that registers the multi-
// client SKILL.md walk rule plus the .skillport/skills global dir.
// No presence (Skillport is a skill-source tag, not an installable).
var projectSkills = client{
	directRules: []parseRule{
		{target: filepath.Join(homeDir, ".skillport/skills"), parse: parseSkillDir("skillport")},
	},
	walkRules: []parseRule{
		{target: "SKILL.md", parse: parseProjectSkill},
	},
	walkSkipPrefixes: []string{".skillport/skills"},
}

// skillFrontmatter is the subset of SKILL.md frontmatter devicescan
// records. Declaring only these two fields means yaml.v3 silently
// ignores everything else (nested metadata, sequence-valued vendor
// fields, etc.) instead of failing the parse.
type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// parseSkillFrontmatter extracts the leading `---`-delimited YAML
// block. Returns ok=false only when an opening delimiter has no
// matching close; no frontmatter at all returns zero-value fields and
// ok=true (caller falls back to directory-derived metadata).
func parseSkillFrontmatter(content string) (skillFrontmatter, bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return skillFrontmatter{}, true
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return skillFrontmatter{}, false
	}

	var fm skillFrontmatter
	_ = yaml.Unmarshal([]byte(strings.Join(lines[1:endIdx], "\n")), &fm)
	return fm, true
}

// parseSkill reads <skillDir>/SKILL.md and returns the wire skill row
// plus the SKILL.md file. Name falls back to the directory basename
// when frontmatter omits or display-names it. ok=false on
// missing/unreadable SKILL.md or structurally broken frontmatter.
func parseSkill(skillDir, client, projectPath string) (parseResult, bool) {
	var (
		skillPath      = filepath.Join(skillDir, "SKILL.md")
		skillData, err = os.ReadFile(skillPath)
	)
	if err != nil {
		return parseResult{}, false
	}

	fm, ok := parseSkillFrontmatter(string(skillData))
	if !ok {
		log.Debugf("devicescan: skipping malformed skill %q (frontmatter delimiters)", skillPath)
		return parseResult{}, false
	}

	name := fm.Name
	if name == "" {
		name = filepath.Base(skillDir)
	}

	return parseResult{
		files: []types.DeviceScanFile{readScanFile(skillPath)},
		skills: []types.DeviceScanSkill{{
			Client:       client,
			ProjectPath:  projectPath,
			File:         skillPath,
			Name:         name,
			Description:  fm.Description,
			Files:        listArtifactPaths(skillDir, skillExts),
			HasScripts:   dirExists(filepath.Join(skillDir, "scripts")),
			GitRemoteURL: readGitOrigin(skillDir),
		}},
	}, true
}

// parseSkillDir returns a parseFunc that enumerates immediate-child
// directories and emits one skill per child containing SKILL.md. Used
// by claude_code, codex, opencode, and skillport for their respective
// global skill directories.
func parseSkillDir(client string) parseFunc {
	return func(dirPath string) parseResult {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return parseResult{}
		}

		var out parseResult
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			skillDir := filepath.Join(dirPath, e.Name())
			if !fileExists(filepath.Join(skillDir, "SKILL.md")) {
				continue
			}

			sub, ok := parseSkill(skillDir, client, "")
			if !ok {
				continue
			}

			out.merge(sub)
		}
		return out
	}
}

// parseProjectSkill is the parseFunc for the SKILL.md walk pattern.
// Tags the skill by inferring its owning client from the first
// home-relative path component (e.g. .cursor/... → "cursor"); falls
// back to multiClient with the skill directory as the project path.
//
// Skills owned by direct paths are not re-emitted here because each
// real client registers its global skill / plugin subtrees as
// walkSkipPrefixes, so the central walk never visits them.
func parseProjectSkill(path string) parseResult {
	if filepath.Base(path) != "SKILL.md" {
		return parseResult{}
	}

	var (
		skillDir    = filepath.Dir(path)
		client      = multiClient
		projectPath = skillDir
	)
	if rel, err := filepath.Rel(homeDir, path); err == nil {
		first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if name, ok := homeDirClients[first]; ok {
			client = name
			projectPath = ""
		}
	}

	sub, _ := parseSkill(skillDir, client, projectPath)
	return sub
}
