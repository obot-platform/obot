package cli

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/cli/internal/mcplogin"
	"github.com/spf13/cobra"
)

type MCPLogin struct {
	URL string `usage:"Pending UI authentication URL from Obot"`
}

func (m *MCPLogin) Customize(cmd *cobra.Command) {
	cmd.Use = "login --url <URL>"
	cmd.Short = "Authenticate with an MCP server using a local OAuth callback"
	cmd.Args = cobra.NoArgs
}

func (m *MCPLogin) Run(cmd *cobra.Command, _ []string) error {
	if m.URL == "" {
		return fmt.Errorf("--url is required")
	}
	if err := mcplogin.Run(cmd.Context(), m.URL); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "MCP login complete. Return to Obot to continue.")
	return nil
}
