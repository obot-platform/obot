package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/acorn-io/acorn/pkg/cli/invokeclient"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Invoke struct {
	Thread  string `usage:"Thread name to run the agent in." short:"t"`
	Step    string `usage:"Workflow step to rerun from, thread is already required" short:"s"`
	Quiet   *bool  `usage:"Only print output characters" short:"q"`
	Verbose bool   `usage:"Print more information" short:"v"`
	Async   bool   `usage:"Run the agent asynchronously" short:"a"`
	root    *Acorn
}

func (l *Invoke) GetQuiet() bool {
	if l.Quiet == nil {
		return false
	}
	return *l.Quiet
}

func (l *Invoke) Pre(*cobra.Command, []string) error {
	if l.Quiet == nil && term.IsTerminal(int(os.Stdout.Fd())) {
		l.Quiet = new(bool)
	}
	return nil
}

func (l *Invoke) Customize(cmd *cobra.Command) {
	cmd.Use = "invoke [flags] AGENT [INPUT...]"
	cmd.Args = cobra.MinimumNArgs(1)
	cmd.Flags().SetInterspersed(false)
}

func (l *Invoke) Run(cmd *cobra.Command, args []string) error {
	if l.Step != "" && l.Thread == "" && !system.IsThreadID(args[0]) {
		return fmt.Errorf("thread is required when rerunning a step")
	}

	return invokeclient.Invoke(cmd.Context(), l.root.Client, args[0], strings.Join(args[1:], " "), invokeclient.Options{
		ThreadID: l.Thread,
		Quiet:    l.GetQuiet(),
		Details:  l.Verbose,
		Async:    l.Async,
		Step:     l.Step,
	})
}
