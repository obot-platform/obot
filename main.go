package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	_ "time/tzdata"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/pkg/cli"
)

func main() {
	// Don't shutdown on SIGTERM, only on SIGINT. SIGTERM is handled by the controller leader election
	cmd.ShutdownSignals = []os.Signal{os.Interrupt}
	root := cli.New()
	if err := root.ExecuteContext(cmd.SetupSignalContext()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			os.Exit(1)
		}
		if cli.ErrorAlreadyReported(err) {
			os.Exit(1)
		}
		// Not stdlib log: its slog bridge would demote this to INFO with no
		// source, and this is the process's last word.
		slog.Error("exiting", "error", err)
		os.Exit(1)
	}
}
