package cli

import (
	"github.com/obot-platform/obot/pkg/cli/internal/mcpconnect"
	"github.com/spf13/cobra"
)

type MCPConnect struct{}

func (*MCPConnect) Customize(cmd *cobra.Command) {
	cmd.Use = "connect <URL>"
	cmd.Short = "Connect a local STDIO MCP client to the Obot gateway"
	cmd.Args = cobra.ExactArgs(1)
}

func (*MCPConnect) Run(cmd *cobra.Command, args []string) error {
	return mcpconnect.Run(cmd.Context(), args[0])
}
