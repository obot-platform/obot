package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type Login struct {
	NoExpiration bool `usage:"Set the token to never expire"`
	ForceRefresh bool `usage:"Force refresh the token even if a valid one is cached"`
	PrintToken   bool `usage:"Print the token to stdout after logging in"`
	root         *Obot
}

func (l *Login) Customize(cmd *cobra.Command) {
	cmd.Use = "login"
	cmd.Short = "Authenticate with an Obot server and store credentials locally"
	cmd.Args = cobra.NoArgs
}

func (l *Login) Run(cmd *cobra.Command, _ []string) error {
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
