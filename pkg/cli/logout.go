package cli

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/spf13/cobra"
)

type Logout struct {
	root *Obot
}

func (l *Logout) Customize(cmd *cobra.Command) {
	cmd.Use = "logout"
	cmd.Short = "Remove locally stored Obot credentials"
	cmd.Args = cobra.NoArgs
}

func (l *Logout) Run(_ *cobra.Command, _ []string) error {
	removed, err := internal.Logout()
	if err != nil {
		return err
	}
	if !removed {
		fmt.Println("No stored credentials found")
		return nil
	}
	fmt.Println("Logged out")
	return nil
}
