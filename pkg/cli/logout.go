package cli

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/spf13/cobra"
)

type Logout struct {
	URL  string `usage:"Obot app URL whose stored credentials should be removed"`
	root *Obot
}

func (l *Logout) Customize(cmd *cobra.Command) {
	cmd.Use = "logout"
	cmd.Short = "Remove locally stored Obot credentials"
	cmd.Args = cobra.NoArgs
}

func (l *Logout) Run(cmd *cobra.Command, _ []string) error {
	appURL := l.URL
	if appURL == "" {
		var err error
		appURL, err = internal.AppURLForAPIBaseURL(l.root.Client.BaseURL)
		if err != nil {
			return err
		}
	} else {
		var err error
		appURL, err = localconfig.NormalizeAppURL(appURL)
		if err != nil {
			return err
		}
	}

	removed, err := internal.Logout(appURL)
	if err != nil {
		return err
	}
	if !removed {
		fmt.Fprintln(cmd.OutOrStdout(), "No stored credentials found")
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Logged out")
	return nil
}
