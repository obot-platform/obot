package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/otto8-ai/otto8/apiclient"
	"github.com/spf13/cobra"
)

type Threads struct {
	root  *Otto8
	Quiet bool `usage:"Only print IDs of threads" short:"q"`
	Wide  bool `usage:"Print more information" short:"w"`
}

func (l *Threads) Customize(cmd *cobra.Command) {
	cmd.Use = "threads [flags] AGENT_ID"
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"thread", "t"}
}

func (l *Threads) Run(cmd *cobra.Command, args []string) error {
	var opts apiclient.ListThreadsOptions
	if len(args) > 0 {
		opts.AgentID = args[0]
	}
	threads, err := l.root.Client.ListThreads(cmd.Context(), opts)
	if err != nil {
		return err
	}

	if l.Quiet {
		for _, thread := range threads.Items {
			fmt.Println(thread.ID)
		}
		return nil
	}

	w := newTable("ID", "PARENT_THREAD", "DESC", "AGENT/WF", "CURRENT/LASTRUN", "STATE", "CREATED")
	for _, thread := range threads.Items {
		agentWF := thread.AgentID
		if agentWF == "" {
			agentWF = thread.WorkflowID
		}
		run := thread.CurrentRunID
		if run == "" {
			run = thread.LastRunID
		}
		w.WriteRow(thread.ID, thread.ParentThreadID, thread.Description, agentWF, run, thread.State, humanize.Time(thread.Created.Time))
	}

	return w.Err()
}
