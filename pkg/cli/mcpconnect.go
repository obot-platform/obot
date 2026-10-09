package cli

import (
	"github.com/obot-platform/obot/pkg/cli/internal/mcpconnect"
	"github.com/spf13/cobra"
)

type MCPConnect struct {
	callbackPaths []string
}

func (m *MCPConnect) Customize(cmd *cobra.Command) {
	cmd.Use = "connect <URL>"
	cmd.Short = "Connect a local STDIO MCP client to the Obot gateway"
	cmd.Args = cobra.ExactArgs(1)
	cmd.Flags().StringArrayVar(&m.callbackPaths, "callback-path", nil, "Allowed provider OAuth callback path (repeatable; defaults to /oauth/callback)")
}

func (m *MCPConnect) Run(cmd *cobra.Command, args []string) error {
	return mcpconnect.Run(cmd.Context(), args[0], m.callbackPaths...)
}
