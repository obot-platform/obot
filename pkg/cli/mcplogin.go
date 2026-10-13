package cli

import (
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/cli/internal/mcplogin"
	"github.com/spf13/cobra"
)

type MCPLogin struct{}

func (m *MCPLogin) Customize(cmd *cobra.Command) {
	cmd.Use = "login <URL>"
	cmd.Short = "Authenticate with an MCP server using a local OAuth callback"
	cmd.Args = cobra.ExactArgs(1)
}

func (m *MCPLogin) Run(cmd *cobra.Command, args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("authentication URL is required")
	}
	if err := mcplogin.Run(cmd.Context(), strings.TrimSpace(args[0])); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "MCP login complete. Return to Obot to continue.")
	return nil
}
