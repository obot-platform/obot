package localagents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

	home, err := resolveHome("", c.home)
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	if binary, err := exec.LookPath("claude"); err == nil && binary != "" {
		result.State = DetectionPresent
		result.Reason = "found claude binary at " + binary
		return result
	}

	configPath := filepath.Join(home, ".claude")
	if fi, err := os.Stat(configPath); err == nil && fi.IsDir() {
		result.State = DetectionPresent
		result.Reason = "found Claude Code config at " + configPath
		return result
	}

	result.Reason = "Claude Code was not detected"
	return result
}

func (c ClaudeCode) InstallBootstrap(ctx context.Context, home string) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := assets.RenderAgentSkills(assets.ClaudeCodeTemplateData())
	if err != nil {
		return InstallResult{}, err
	}

	installed, err := installBootstrapAssets(claudeCodeSkillsRoot(home), rendered)
	if err != nil {
		return InstallResult{}, err
	}

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
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}
	name, installed, err := installSkillArchiveToRoot(claudeCodeSkillsRoot(home), skill)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s for Claude Code", name),
	}, nil
}

func claudeCodeSkillsRoot(home string) string {
	return filepath.Join(home, ".claude", "skills")
}
