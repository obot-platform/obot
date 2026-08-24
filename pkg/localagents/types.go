package localagents

import (
	"context"
)

const (
	DetectionMissing DetectionState = "missing"
	DetectionPresent DetectionState = "present"
)

type DetectionState string

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

type SetupTarget interface {
	ID() string
	DisplayName() string
	InstallBootstrap(ctx context.Context, home string) (InstallResult, error)
}
