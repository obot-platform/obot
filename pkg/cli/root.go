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
	Debug   bool   `usage:"Enable debug logging"`
	BaseURL string `usage:"Base URL for the OBOT API" default:"http://localhost:8080/api"`
	Client  *apiclient.Client
}

func (a *Obot) PersistentPre(*cobra.Command, []string) error {
	if os.Getenv("NO_COLOR") != "" || !term.IsTerminal(int(os.Stdout.Fd())) {
		color.NoColor = true
	}

	if a.Debug {
		logger.SetDebug()
	}

	// Update the client's BaseURL from the flag/env value
	a.Client.BaseURL = a.BaseURL

	if a.Client.Token == "" {
		a.Client = a.Client.WithTokenFetcher(internal.Token)
	}

	return nil
}

func New() *cobra.Command {
	root := &Obot{
		BaseURL: env.VarOrDefault("OBOT_BASE_URL", "http://localhost:8080/api"),
		Client: &apiclient.Client{
			Token: os.Getenv("OBOT_TOKEN"),
		},
	}
	return cmd.Command(root,
		&Server{},
		&Token{root: root},
		&Version{},
	)
}

func (a *Obot) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
