package localagents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obot-platform/obot/pkg/devicescan"
	"github.com/obot-platform/obot/pkg/localagents/assets"
)

const (
	ClaudeCodeAgentID     = "claude-code"
	claudeCodeDisplayName = "Claude Code"
)

type ClaudeCode struct {
	home string
}

func NewClaudeCode() ClaudeCode {
	return ClaudeCode{}
}

func (c ClaudeCode) ID() string {
	return ClaudeCodeAgentID
}

func (c ClaudeCode) DisplayName() string {
	return claudeCodeDisplayName
}

func (c ClaudeCode) Detect(ctx context.Context) DetectionResult {
	result := DetectionResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		State:       DetectionMissing,
	}
	if err := ctx.Err(); err != nil {
		result.Reason = err.Error()
		return result
	}

	home, err := c.resolveHome("")
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	presence := devicescan.DetectClaudeCodePresence(home)
	switch {
	case presence.BinaryPath != "":
		result.State = DetectionPresent
		result.Reason = "found claude binary at " + presence.BinaryPath
	case presence.ConfigPath != "":
		result.State = DetectionPresent
		result.Reason = "found Claude Code config at " + presence.ConfigPath
	case presence.InstallPath != "":
		result.State = DetectionPresent
		result.Reason = "found Claude Code install at " + presence.InstallPath
	default:
		result.Reason = "Claude Code was not detected"
	}

	return result
}

func (c ClaudeCode) InstallBootstrap(ctx context.Context, home string) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := c.resolveHome(home)
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := assets.RenderClaudeSkills(assets.ClaudeCodeTemplateData())
	if err != nil {
		return InstallResult{}, err
	}

	skillsRoot := claudeCodeSkillsRoot(home)
	bySkill := make(map[string][]installFile, len(rendered))
	for _, asset := range rendered {
		prefix := asset.SkillName + "/"
		if !strings.HasPrefix(asset.RelPath, prefix) {
			return InstallResult{}, fmt.Errorf("bootstrap asset path %s is not under %s", asset.RelPath, asset.SkillName)
		}
		rel := strings.TrimPrefix(asset.RelPath, prefix)
		bySkill[asset.SkillName] = append(bySkill[asset.SkillName], installFile{
			RelPath: rel,
			Content: asset.Content,
			Mode:    0644,
		})
	}

	skillNames := make([]string, 0, len(bySkill))
	for skillName := range bySkill {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)

	installed := make([]string, 0, len(rendered))
	for _, skillName := range skillNames {
		target := filepath.Join(skillsRoot, skillName)
		if err := replaceDir(target, bySkill[skillName]); err != nil {
			return InstallResult{}, fmt.Errorf("install %s bootstrap skill: %w", skillName, err)
		}
		for _, file := range bySkill[skillName] {
			installed = append(installed, filepath.Join(target, filepath.FromSlash(file.RelPath)))
		}
	}
	sort.Strings(installed)

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     "Installed Obot bootstrap skills for Claude Code",
	}, nil
}

func (c ClaudeCode) InstallSkill(ctx context.Context, home string, skill SkillArchive) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := c.resolveHome(home)
	if err != nil {
		return InstallResult{}, err
	}
	name, err := skill.installName()
	if err != nil {
		return InstallResult{}, err
	}
	if err := skill.validateFiles(); err != nil {
		return InstallResult{}, err
	}

	files := make([]installFile, 0, len(skill.Files))
	for _, file := range skill.Files {
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil {
			return InstallResult{}, err
		}
		files = append(files, installFile{
			RelPath: rel,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].RelPath < files[j].RelPath
	})

	target := filepath.Join(claudeCodeSkillsRoot(home), name)
	if err := replaceDir(target, files); err != nil {
		return InstallResult{}, fmt.Errorf("install %s skill: %w", name, err)
	}

	installed := make([]string, 0, len(files))
	for _, file := range files {
		installed = append(installed, filepath.Join(target, filepath.FromSlash(file.RelPath)))
	}
	sort.Strings(installed)

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s for Claude Code", name),
	}, nil
}

func (c ClaudeCode) resolveHome(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if strings.TrimSpace(c.home) != "" {
		return c.home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return home, nil
}

func claudeCodeSkillsRoot(home string) string {
	return filepath.Join(home, ".claude", "skills")
}
