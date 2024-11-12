package cli

import (
	"errors"
	"fmt"

	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/spf13/cobra"
)

type Delete struct {
	root *Otto8
}

func (l *Delete) Customize(cmd *cobra.Command) {
	cmd.Use = "delete [flags] ID..."
	cmd.Aliases = []string{"rm", "del"}
}

func (l *Delete) Run(cmd *cobra.Command, args []string) error {
	var errs []error
	for _, id := range args {
		if len(id) < 1 {
			continue
		}

		switch {
		case system.IsThreadID(id):
			if err := l.root.Client.DeleteThread(cmd.Context(), id); err != nil {
				errs = append(errs, err)
			} else {
				fmt.Printf("Thread deleted: %s\n", id)
			}
		case system.IsAgentID(id):
			if err := l.root.Client.DeleteAgent(cmd.Context(), id); err != nil {
				errs = append(errs, err)
			} else {
				fmt.Printf("Agent deleted: %s\n", id)
			}
		case system.IsWorkflowID(id):
			if err := l.root.Client.DeleteWorkflow(cmd.Context(), id); err != nil {
				errs = append(errs, err)
			} else {
				fmt.Printf("Workflow deleted: %s\n", id)
			}
		case system.IsRunID(id):
			if err := l.root.Client.DeleteRun(cmd.Context(), id); err != nil {
				errs = append(errs, err)
			} else {
				fmt.Printf("Workflow deleted: %s\n", id)
			}
		default:
			errs = append(errs, errors.New("invalid ID: "+id))
		}
	}

	return errors.Join(errs...)
}
