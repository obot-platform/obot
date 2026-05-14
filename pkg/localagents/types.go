package localagents

import "context"

type DetectionState string

const (
	DetectionMissing DetectionState = "missing"
	DetectionPresent DetectionState = "present"
)

type DetectionResult struct {
	AgentID     string
	DisplayName string
	State       DetectionState
	Reason      string
}

type InstallResult struct {
	AgentID     string
	DisplayName string
	Installed   []string
	Message     string
}

type Agent interface {
	ID() string
	DisplayName() string
	Detect(ctx context.Context) DetectionResult
}

type DirectInstaller interface {
	Agent
	InstallBootstrap(ctx context.Context, home string) (InstallResult, error)
	InstallSkill(ctx context.Context, home string, skill SkillArchive) (InstallResult, error)
}
