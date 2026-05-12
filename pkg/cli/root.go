package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/gptscript-ai/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
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
		Client: newClient(),
	}
	return cmd.Command(root,
		&Server{},
		&Login{root: root},
		&Logout{root: root},
		&Scan{root: root},
		&Version{},
	)
}

func (a *Obot) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func newClient() *apiclient.Client {
	baseURL := os.Getenv("OBOT_BASE_URL")
	if baseURL == "" {
		if cfg, err := localconfig.Load(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load Obot config: %v\n", err)
		} else if cfg.DefaultURL != "" {
			baseURL = localconfig.APIBaseURL(cfg.DefaultURL)
		}
	}
	if baseURL == "" {
		baseURL = "http://localhost:8080/api"
	}

	return &apiclient.Client{
		BaseURL: baseURL,
		Token:   os.Getenv("OBOT_TOKEN"),
	}
}
