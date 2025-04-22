package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type Token struct {
	root *Obot
}

func (t *Token) Customize(cmd *cobra.Command) {
	cmd.Use = "token"
	cmd.Args = cobra.NoArgs
}

func (t *Token) Run(cmd *cobra.Command, _ []string) error {
	token, err := t.root.Client.GetToken(cmd.Context())
	if err != nil {
		return err
	}
	fmt.Printf("Your token: %s\n", token)
	return nil
}
