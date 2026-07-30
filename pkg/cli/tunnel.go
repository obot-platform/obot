package cli

import (
	"os"
	"os/signal"
	"syscall"

	obottunnel "github.com/obot-platform/obot/pkg/tunnel"
	"github.com/spf13/cobra"
)

// Tunnel connects this machine to an Obot instance so Obot can reach MCP
// servers that are available from this machine's network.
type Tunnel struct {
	Token       string `usage:"Token used to authenticate with the Obot instance"`
	ObotBaseURL string `usage:"Base URL of the Obot instance" default:"http://localhost:8080/api" env:"OBOT_BASE_URL"`
}

func (t *Tunnel) Customize(cmd *cobra.Command) {
	cmd.Use = "tunnel"
	cmd.Short = "Open a tunnel for remote MCP servers"
	cmd.Args = cobra.NoArgs
}

func (t *Tunnel) Run(cmd *cobra.Command, _ []string) error {
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	err := obottunnel.Run(ctx, t.ObotBaseURL, t.Token)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
