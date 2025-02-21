package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type Workflows struct {
	root   *Obot
	Quiet  bool   `usage:"Only print IDs of agents" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
}

func (l *Workflows) Customize(cmd *cobra.Command) {
	cmd.Aliases = []string{"workflow", "wf", "w"}
}

func (l *Workflows) Run(cmd *cobra.Command, args []string) error {
	var (
		wfs types.WorkflowList
		err error
	)

	if len(args) > 0 {
		for _, arg := range args {
			wf, err := l.root.Client.GetWorkflow(cmd.Context(), arg)
			if err != nil {
				return err
			}
			wfs.Items = append(wfs.Items, *wf)
		}
	} else {
		wfs, err = l.root.Client.ListWorkflows(cmd.Context(), apiclient.ListWorkflowsOptions{})
		if err != nil {
			return err
		}
	}

	if ok, err := output(l.Output, wfs); ok || err != nil {
		return err
	}

	if l.Quiet {
		for _, agent := range wfs.Items {
			fmt.Println(agent.ID)
		}
		return nil
	}

	w := newTable("ID", "CREATED")
	for _, wf := range wfs.Items {
		w.WriteRow(wf.ID, humanize.Time(wf.Created.Time))
	}

	return w.Err()
}
