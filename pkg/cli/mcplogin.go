package cli

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/cli/internal/mcpconnect"
	"github.com/spf13/cobra"
)

type MCPLogin struct {
	URL           string `usage:"MCP connection URL to authenticate with"`
	callbackPaths []string
}

func (m *MCPLogin) Customize(cmd *cobra.Command) {
	cmd.Use = "login --url <URL>"
	cmd.Short = "Authenticate with an MCP server using a local OAuth callback"
	cmd.Args = cobra.NoArgs
	cmd.Flags().StringArrayVar(&m.callbackPaths, "callback-path", nil, "Allowed provider OAuth callback path (repeatable; defaults to /oauth/callback)")
}

func (m *MCPLogin) Run(cmd *cobra.Command, _ []string) error {
	if m.URL == "" {
		return fmt.Errorf("--url is required")
	}
	if err := mcpconnect.Login(cmd.Context(), m.URL, m.callbackPaths...); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "MCP login complete. Return to Obot to continue.")
	return nil
}
