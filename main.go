package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	_ "time/tzdata"

	gptcmd "github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/embedded"
	"github.com/obot-platform/nanobot/pkg/supervise"
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
	gptcmd.ShutdownSignals = []os.Signal{os.Interrupt}
	root := cli.New()
	if err := root.ExecuteContext(gptcmd.SetupSignalContext()); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			os.Exit(1)
		}
		if cli.ErrorAlreadyReported(err) {
			os.Exit(1)
		}
		log.Fatal(err)
	}
}
