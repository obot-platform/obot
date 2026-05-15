package assets

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"
	"text/template"
)

//go:embed claude/skills/*/SKILL.md.tmpl
var templateFS embed.FS

var claudeSkillTemplates = []string{
	"claude/skills/obot/SKILL.md.tmpl",
	"claude/skills/obot-search-skills/SKILL.md.tmpl",
	"claude/skills/obot-install-skill/SKILL.md.tmpl",
	"claude/skills/obot-scan/SKILL.md.tmpl",
}

// TemplateData is the agent-specific data used to render bootstrap
// assets.
type TemplateData struct {
	AgentID         string
	InstallAgentArg string
}

// SkillAsset is one rendered skill file relative to an agent skills
// root directory.
type SkillAsset struct {
	SkillName string
	RelPath   string
	Content   []byte
}

// ClaudeCodeTemplateData returns the template data for direct Claude
// Code installs.
func ClaudeCodeTemplateData() TemplateData {
	return TemplateData{
		AgentID:         "claude-code",
		InstallAgentArg: "claude-code",
	}
}

// CursorTemplateData returns the template data for direct Cursor installs.
func CursorTemplateData() TemplateData {
	return TemplateData{
		AgentID:         "cursor",
		InstallAgentArg: "cursor",
	}
}

// RenderAgentSkills renders all Obot bootstrap skill assets.
func RenderAgentSkills(data TemplateData) ([]SkillAsset, error) {
	if err := validateTemplateData(data); err != nil {
		return nil, err
	}

	assets := make([]SkillAsset, 0, len(claudeSkillTemplates))
	for _, templatePath := range claudeSkillTemplates {
		content, err := renderTemplate(templatePath, data)
		if err != nil {
			return nil, err
		}

		skillName := path.Base(path.Dir(templatePath))
		assets = append(assets, SkillAsset{
			SkillName: skillName,
			RelPath:   path.Join(skillName, "SKILL.md"),
			Content:   content,
		})
	}

	sort.SliceStable(assets, func(i, j int) bool {
		return assets[i].RelPath < assets[j].RelPath
	})
	return assets, nil
}

func validateTemplateData(data TemplateData) error {
	if strings.TrimSpace(data.AgentID) == "" {
		return fmt.Errorf("agent ID is required")
	}
	if strings.TrimSpace(data.InstallAgentArg) == "" {
		return fmt.Errorf("install agent arg is required")
	}
	return nil
}

func renderTemplate(templatePath string, data TemplateData) ([]byte, error) {
	tmpl, err := template.New(path.Base(templatePath)).
		Option("missingkey=error").
		ParseFS(templateFS, templatePath)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", templatePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, path.Base(templatePath), data); err != nil {
		return nil, fmt.Errorf("render %s: %w", templatePath, err)
	}
	return buf.Bytes(), nil
}
