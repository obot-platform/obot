package cli

import (
	"fmt"
	"os"

	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/spf13/cobra"
)

type Login struct {
	NoExpiration bool   `usage:"Set the token to never expire"`
	ForceRefresh bool   `usage:"Force refresh the token even if a valid one is cached"`
	PrintToken   bool   `usage:"Print the token to stdout after logging in"`
	URL          string `usage:"Obot app URL to authenticate against"`
	root         *Obot
}

func (l *Login) Customize(cmd *cobra.Command) {
	cmd.Use = "login"
	cmd.Short = "Authenticate with an Obot server and store credentials locally"
	cmd.Args = cobra.NoArgs
}

func (l *Login) Run(cmd *cobra.Command, _ []string) error {
	if l.URL != "" {
		appURL, err := localconfig.NormalizeAppURL(l.URL)
		if err != nil {
			return err
		}
		l.root.Client.BaseURL = localconfig.APIBaseURL(appURL)
	}

	token, err := l.root.Client.GetToken(cmd.Context(), l.NoExpiration, l.ForceRefresh)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Logged in to %s\n", l.root.Client.BaseURL)
	if l.PrintToken {
		fmt.Println(token)
	}
	return nil
}
