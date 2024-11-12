package cli

import (
	"fmt"
	"iter"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/otto8-ai/otto8/apiclient"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type Runs struct {
	root   *Otto8
	Wide   bool `usage:"Print more information" short:"w"`
	Quiet  bool `usage:"Only print IDs of runs" short:"q"`
	Follow bool `usage:"Follow the output of runs" short:"f"`
}

func (l *Runs) Customize(cmd *cobra.Command) {
	cmd.Use = "runs [flags] [AGENT_ID] [THREAD_ID]"
	cmd.Args = cobra.MaximumNArgs(2)
	cmd.Aliases = []string{"run", "r"}
}

func (l *Runs) printRunsQuiet(i iter.Seq[types.Run]) error {
	for run := range i {
		fmt.Println(run.ID)
	}
	return nil
}

func (l *Runs) printRuns(i iter.Seq[types.Run], flush bool) error {
	w := newTable("ID", "PREV", "AGENT/WF", "THREAD", "STEP", "STATE", "INPUT", "OUTPUT", "CREATED")
	for run := range i {
		run.Input = truncate(run.Input, l.Wide)
		run.Output = truncate(run.Output, l.Wide)
		run.Error = truncate(run.Error, l.Wide)
		if run.Error != "" {
			run.Output = run.Error
		}
		agentWF := run.AgentID
		if run.AgentID == "" {
			agentWF = run.WorkflowID
		}

		out := run.Output
		if run.SubCallWorkflowID != "" {
			out = "Workflow: " + run.SubCallWorkflowID + " ,Input: " + run.SubCallInput
		}

		w.WriteRow(run.ID, run.PreviousRunID, agentWF, run.ThreadID, run.WorkflowStepID, run.State, run.Input, out, humanize.Time(run.Created.Time))
		if flush {
			w.Flush()
		}
	}

	return w.Err()
}

func chanToIter[T any](c <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for item := range c {
			if !yield(item) {
				go func() {
					// drain
					for range c {
					}
				}()
				return
			}
		}
	}
}

func sliceToIter[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, item := range s {
			if !yield(item) {
				return
			}
		}
	}
}

func (l *Runs) Run(cmd *cobra.Command, args []string) error {
	var (
		opts  apiclient.ListRunsOptions
		flush bool
		list  iter.Seq[types.Run]
	)
	if len(args) > 0 {
		opts.AgentID = args[0]
	}
	if len(args) > 1 {
		opts.ThreadID = args[1]
	}

	if l.Follow {
		items, err := l.root.Client.StreamRuns(cmd.Context(), opts)
		if err != nil {
			return err
		}
		list = chanToIter(items)
		flush = true
	} else {
		runs, err := l.root.Client.ListRuns(cmd.Context(), opts)
		if err != nil {
			return err
		}
		list = sliceToIter(runs.Items)
	}

	if l.Quiet {
		return l.printRunsQuiet(list)
	}

	return l.printRuns(list, flush)
}

func truncate(text string, wide bool) string {
	if wide {
		return text
	}
	text = strings.Split(text, "\n")[0]
	maxLength := pterm.GetTerminalWidth() / 3
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
}
