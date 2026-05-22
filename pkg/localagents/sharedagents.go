package localagents

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/obot-platform/obot/pkg/localagents/assets"
)

const (
	SharedAgentsID          = "agents"
	sharedAgentsDisplayName = "All clients that support ~/.agents"
)

type SharedAgents struct {
	home string
}

func NewSharedAgents() SharedAgents {
	return SharedAgents{}
}

func (a SharedAgents) ID() string {
	return SharedAgentsID
}

func (a SharedAgents) DisplayName() string {
	return sharedAgentsDisplayName
}

func (a SharedAgents) InstallBootstrap(ctx context.Context, home string) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, a.home)
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := assets.RenderAgentSkills(assets.SharedAgentsTemplateData())
	if err != nil {
		return InstallResult{}, err
	}

	installed, err := installBootstrapAssets(sharedAgentsSkillsRoot(home), rendered)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     a.ID(),
		DisplayName: a.DisplayName(),
		Installed:   installed,
		Message:     "Installed Obot bootstrap skills for All clients that support ~/.agents",
	}, nil
}

func (a SharedAgents) InstallSkill(ctx context.Context, home string, skill SkillArchive) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, a.home)
	if err != nil {
		return InstallResult{}, err
	}
	name, installed, err := installSkillArchiveToRoot(sharedAgentsSkillsRoot(home), skill)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     a.ID(),
		DisplayName: a.DisplayName(),
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s for All clients that support ~/.agents", name),
	}, nil
}

func sharedAgentsSkillsRoot(home string) string {
	return filepath.Join(home, ".agents", "skills")
}
