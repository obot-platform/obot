package cli

import (
	"fmt"

	"github.com/acorn-io/acorn/apiclient"
	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

type Threads struct {
	root   *Acorn
	Quiet  bool   `usage:"Only print IDs of threads" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
}

func (l *Threads) Customize(cmd *cobra.Command) {
	cmd.Use = "threads [flags]"
	cmd.Aliases = []string{"thread", "t"}
}

func (l *Threads) Run(cmd *cobra.Command, args []string) error {
	var (
		threads types.ThreadList
		err     error
	)
	if len(args) > 0 {
		for _, arg := range args {
			thread, err := l.root.Client.GetThread(cmd.Context(), arg)
			if err != nil {
				return err
			}
			threads.Items = append(threads.Items, *thread)
		}
	} else {
		threads, err = l.root.Client.ListThreads(cmd.Context(), apiclient.ListThreadsOptions{})
		if err != nil {
			return err
		}
	}

	if ok, err := output(l.Output, threads); ok || err != nil {
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
