package cli

import (
	"github.com/otto8-ai/otto8/apiclient"
	"github.com/otto8-ai/otto8/pkg/cli/events"
	"github.com/spf13/cobra"
)

type ThreadPrint struct {
	root    *Otto8
	Quiet   bool `usage:"Only print response content of threads" short:"q"`
	Follow  bool `usage:"Follow the thread and print new events" short:"f"`
	Verbose bool `usage:"Print more information" short:"v"`
}

func (l *ThreadPrint) Customize(cmd *cobra.Command) {
	cmd.Use = "print [flags] [THREAD_ID]"
	cmd.Args = cobra.ExactArgs(1)
}

func (l *ThreadPrint) Run(cmd *cobra.Command, args []string) error {
	var (
		printer = events.NewPrinter(cmd.Context(), l.root.Client, l.Quiet, l.Verbose)
	)

	events, err := l.root.Client.ThreadEvents(cmd.Context(), args[0], apiclient.ThreadEventsOptions{
		Follow: l.Follow,
	})
	if err != nil {
		return err
	}

	return printer.Print("", events)
}
