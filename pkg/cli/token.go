package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type Token struct {
	NoExpiration bool `usage:"Set the token to never expire"`
	ForceRefresh bool `usage:"Force refresh the token"`
	root         *Obot
}

func (t *Token) Customize(cmd *cobra.Command) {
	cmd.Use = "token"
	cmd.Args = cobra.NoArgs
}

func (t *Token) Run(cmd *cobra.Command, _ []string) error {
	token, err := t.root.Client.GetToken(cmd.Context(), t.NoExpiration, t.ForceRefresh)
	if err != nil {
		return err
	}
	fmt.Println(token)
	return nil
}
