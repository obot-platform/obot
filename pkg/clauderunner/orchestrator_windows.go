package clauderunner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/pkg/gateway/client"
)

// StartOrchestrator requires the Unix spawn hooks and process groups used by the container runtime.
func StartOrchestrator(context.Context, string, string, string, string) (func(), error) {
	return nil, fmt.Errorf("the Claude orchestrator is not supported on Windows")
}

// ManageOrchestrators cannot supervise the container-only orchestrator on Windows.
func ManageOrchestrators(context.Context, *client.Client, string) func() {
	slog.Warn("The Claude orchestrator is not supported on Windows")

	return func() {}
}
