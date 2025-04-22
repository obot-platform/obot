package cli

import (
	"os"

	"github.com/fatih/color"
	"github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/env"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Obot struct {
	Debug  bool `usage:"Enable debug logging"`
	Client *apiclient.Client
}

func (a *Obot) PersistentPre(*cobra.Command, []string) error {
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
	root := &Obot{
		Client: &apiclient.Client{
			BaseURL: env.VarOrDefault("OBOT_BASE_URL", "http://localhost:8080/api"),
			Token:   os.Getenv("OBOT_TOKEN"),
		},
	}
	return cmd.Command(root,
		&Create{root: root},
		&Agents{root: root},
		cmd.Command(&Obots{root: root},
			&CreateObot{root: root},
			&DeleteObot{root: root}),
		&Catalog{root: root},
		&Edit{root: root},
		&Update{root: root},
		&Delete{root: root},
		&Invoke{root: root},
		&Tasks{root: root},
		cmd.Command(&Threads{root: root}, &ThreadPrint{root: root}),
		cmd.Command(&Credentials{root: root}, &CredentialsDelete{root: root}),
		cmd.Command(&Runs{root: root}, &Debug{root: root}, &RunPrint{root: root}),
		cmd.Command(&Tools{root: root},
			&ToolUnregister{root: root},
			&ToolRegister{root: root},
			&ToolUpdate{root: root}),
		&Server{},
		&Token{root: root},
		&Version{},
	)
}

func (a *Obot) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
