package cli

import (
	"github.com/acorn-io/acorn/pkg/cli/events"
	"github.com/spf13/cobra"
)

type WorkflowAuth struct {
	root *Acorn
}

func (l *WorkflowAuth) Customize(cmd *cobra.Command) {
	cmd.Use = "authenticate [flags] WORKFLOW_ID"
	cmd.Aliases = []string{"auth", "login"}
	cmd.Args = cobra.ExactArgs(1)
}

func (l *WorkflowAuth) Run(cmd *cobra.Command, args []string) error {
	resp, err := l.root.Client.AuthenticateWorkflow(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	printer := events.NewPrinter(cmd.Context(), l.root.Client, false, false)
	return printer.Print(resp.Events)
}
