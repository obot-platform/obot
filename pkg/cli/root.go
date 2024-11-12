package cli

import (
	"os"

	"github.com/fatih/color"
	"github.com/gptscript-ai/cmd"
	"github.com/gptscript-ai/gptscript/pkg/env"
	"github.com/otto8-ai/otto8/apiclient"
	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/cli/internal"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Otto8 struct {
	Debug  bool `usage:"Enable debug logging"`
	Client *apiclient.Client
}

func (a *Otto8) PersistentPre(cmd *cobra.Command, args []string) error {
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		color.NoColor = true
	}

	if a.Debug {
		logger.SetDebug()
	}

	if a.Client.Token == "" && cmd.Use != "server" && cmd.Use != "otto8" && cmd.Use != "version" {
		token, err := internal.Token(cmd.Context(), a.Client.BaseURL, "otto8")
		if err != nil {
			return err
		}

		a.Client = a.Client.WithToken(token)
	}

	return nil
}

func New() *cobra.Command {
	root := &Otto8{
		Client: &apiclient.Client{
			BaseURL: env.VarOrDefault("OTTO8_BASE_URL", "http://localhost:8080/api"),
			Token:   os.Getenv("OTTO8_TOKEN"),
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
			&StepTemplateUpdate{root: root}),
		&Server{},
		&Version{},
	)
}

func (a *Otto8) Run(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}
