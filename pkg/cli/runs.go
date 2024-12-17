package cli

import (
	"fmt"
	"iter"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type Runs struct {
	root   *Obot
	Wide   bool   `usage:"Print more information" short:"w"`
	Quiet  bool   `usage:"Only print IDs of runs" short:"q"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
	Follow bool   `usage:"Follow the output of runs" short:"f"`
}

func (l *Runs) Customize(cmd *cobra.Command) {
	cmd.Use = "runs [flags]"
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
					//nolint:revive
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

	if l.Follow {
		items, err := l.root.Client.StreamRuns(cmd.Context(), opts)
		if err != nil {
			return err
		}
		list = chanToIter(items)
		flush = true
	} else {
		var (
			runs types.RunList
			err  error
		)
		if len(args) > 0 {
			for _, arg := range args {
				run, err := l.root.Client.GetRun(cmd.Context(), arg)
				if err != nil {
					return err
				}
				runs.Items = append(runs.Items, *run)
			}
		} else {
			runs, err = l.root.Client.ListRuns(cmd.Context(), opts)
			if err != nil {
				return err
			}
		}
		if ok, err := output(l.Output, runs); ok || err != nil {
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
