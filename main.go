package main

import (
	"fmt"
	"os"
	_ "time/tzdata"

	"github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/embedded"
	"github.com/nanobot-ai/nanobot/pkg/supervise"
	"github.com/obot-platform/obot/pkg/cli"
)

func main() {
	if os.Getenv("GPTSCRIPT_EMBEDDED") != "false" {
		if embedded.Run(embedded.Options{}) {
			return
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "_exec" {
		if err := supervise.Daemon(); err != nil {
			fmt.Printf("failed to run nanobot daemon: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	// Don't shutdown on SIGTERM, only on SIGINT. SIGTERM is handled by the controller leader election
	cmd.ShutdownSignals = []os.Signal{os.Interrupt}
	cmd.Main(cli.New())
}
