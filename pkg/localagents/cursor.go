package localagents

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/obot-platform/obot/pkg/devicescan"
	"github.com/obot-platform/obot/pkg/localagents/assets"
)

const (
	CursorAgentID     = "cursor"
	cursorDisplayName = "Cursor"
)

type Cursor struct {
	home string
}

func NewCursor() Cursor {
	return Cursor{}
}

func (c Cursor) ID() string {
	return CursorAgentID
}

func (c Cursor) DisplayName() string {
	return cursorDisplayName
}

func (c Cursor) Detect(ctx context.Context) DetectionResult {
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

	presence := devicescan.DetectCursorPresence(home)
	switch {
	case presence.BinaryPath != "":
		result.State = DetectionPresent
		result.Reason = "found cursor binary at " + presence.BinaryPath
	case presence.ConfigPath != "":
		result.State = DetectionPresent
		result.Reason = "found Cursor config at " + presence.ConfigPath
	case presence.InstallPath != "":
		result.State = DetectionPresent
		result.Reason = "found Cursor install at " + presence.InstallPath
	default:
		result.Reason = "Cursor was not detected"
	}

	return result
}

func (c Cursor) InstallBootstrap(ctx context.Context, home string) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}

	rendered, err := assets.RenderAgentSkills(assets.CursorTemplateData())
	if err != nil {
		return InstallResult{}, err
	}

	installed, err := installBootstrapAssets(cursorSkillsRoot(home), rendered)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     "Installed Obot bootstrap skills for Cursor",
	}, nil
}

func (c Cursor) InstallSkill(ctx context.Context, home string, skill SkillArchive) (InstallResult, error) {
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	home, err := resolveHome(home, c.home)
	if err != nil {
		return InstallResult{}, err
	}
	name, installed, err := installSkillArchiveToRoot(cursorSkillsRoot(home), skill)
	if err != nil {
		return InstallResult{}, err
	}

	return InstallResult{
		AgentID:     c.ID(),
		DisplayName: c.DisplayName(),
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s for Cursor", name),
	}, nil
}

func cursorSkillsRoot(home string) string {
	return filepath.Join(home, ".cursor", "skills")
}
