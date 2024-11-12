package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

type Debug struct {
	root *Otto8
}

func (l *Debug) Customize(cmd *cobra.Command) {
	cmd.Use = "debug [flags] [RUN_ID]"
	cmd.Args = cobra.ExactArgs(1)
}

func (l *Debug) Run(cmd *cobra.Command, args []string) error {
	debug, err := l.root.Client.DebugRun(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(debug)
}
