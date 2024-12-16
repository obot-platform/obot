package cli

import (
	"os"

	"github.com/acorn-io/acorn/apiclient"
	"github.com/acorn-io/acorn/logger"
	"github.com/acorn-io/acorn/pkg/cli/internal"
	"github.com/fatih/color"
	"github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/env"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Acorn struct {
	Debug  bool `usage:"Enable debug logging"`
	Client *apiclient.Client
}

func (a *Acorn) PersistentPre(*cobra.Command, []string) error {
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		color.NoColor = true
	}

	if a.Debug {
		logger.SetDebug()
	}

	if a.Client.Token == "" {
		a.Client = a.Client.WithTokenFetcher(internal.Token)
	}

	return nil
}

func New() *cobra.Command {
	root := &Acorn{
		Client: &apiclient.Client{
			BaseURL: env.VarOrDefault("ACORN_BASE_URL", "http://localhost:8080/api"),
			Token:   os.Getenv("ACORN_TOKEN"),
		},
	}
	return cmd.Command(root,
		&Create{root: root},
		&Agents{root: root},
		cmd.Command(&Workflows{root: root},
			&WorkflowAuth{root: root}),
		&Edit{root: root},
		&Update{root: root},
		&Delete{root: root},
		&Invoke{root: root},
		cmd.Command(&Threads{root: root}, &ThreadPrint{root: root}),
		cmd.Command(&Credentials{root: root}, &CredentialsDelete{root: root}),
		cmd.Command(&Runs{root: root}, &Debug{root: root}, &RunPrint{root: root}),
		cmd.Command(&Tools{root: root},
			&ToolUnregister{root: root},
			&ToolRegister{root: root},
			&ToolUpdate{root: root}),
		&Webhooks{root: root},
		&Server{},
		&Version{},
	)
}

func (a *Acorn) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
