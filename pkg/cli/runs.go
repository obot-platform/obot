package cli

import (
	"fmt"
	"iter"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gptscript-ai/otto/pkg/api/client"
	"github.com/gptscript-ai/otto/pkg/api/types"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

type Runs struct {
	root   *Otto
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
	w := newTable("ID", "PREV", "AGENT/WF", "THREAD", "STATE", "INPUT", "OUTPUT", "CREATED")
	for run := range i {
		run.Input = truncate(strings.Split(run.Input, "\n")[0], l.Wide)
		run.Output = truncate(strings.Split(run.Output, "\n")[0], l.Wide)
		run.Error = truncate(strings.Split(run.Error, "\n")[0], l.Wide)
		if run.Error != "" {
			run.Output = run.Error
		}
		agentWF := run.AgentID
		if run.AgentID == "" {
			agentWF = run.WorkflowID
		}

		w.WriteRow(run.ID, run.PreviousRunID, agentWF, run.ThreadID, string(run.State), run.Input, run.Output, humanize.Time(run.Created))
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
		opts  []client.ListRunsOptions
		flush bool
		list  iter.Seq[types.Run]
	)
	if len(args) > 0 {
		opts = append(opts, client.ListRunsOptions{
			AgentID: args[0],
		})
	}
	if len(args) > 1 {
		opts = append(opts, client.ListRunsOptions{
			ThreadID: args[1],
		})
	}

	if l.Follow {
		items, err := l.root.Client.StreamRuns(cmd.Context(), opts...)
		if err != nil {
			return err
		}
		list = chanToIter(items)
		flush = true
	} else {
		runs, err := l.root.Client.ListRuns(cmd.Context(), opts...)
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
	maxLength := pterm.GetTerminalWidth() / 3
	if len(text) > maxLength {
		return text[:maxLength] + "..."
	}
	return text
}
