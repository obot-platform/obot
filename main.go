package main

import (
	"os"

	"github.com/acorn-io/acorn/pkg/cli"
	"github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/embedded"
)

func main() {
	if os.Getenv("GPTSCRIPT_EMBEDDED") != "false" {
		if embedded.Run(embedded.Options{}) {
			return
		}
	}
	// Don't shutdown on SIGTERM, only on SIGINT. SIGTERM is handled by the controller leader election
	cmd.ShutdownSignals = []os.Signal{os.Interrupt}
	cmd.Main(cli.New())
}
