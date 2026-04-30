package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type Login struct {
	NoExpiration bool `usage:"Set the token to never expire"`
	root         *Obot
}

func (l *Login) Customize(cmd *cobra.Command) {
	cmd.Use = "login"
	cmd.Short = "Authenticate with an Obot server and store credentials locally"
	cmd.Args = cobra.NoArgs
}

func (l *Login) Run(cmd *cobra.Command, _ []string) error {
	if _, err := l.root.Client.GetToken(cmd.Context(), l.NoExpiration, true); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s\n", l.root.Client.BaseURL)
	return nil
}
