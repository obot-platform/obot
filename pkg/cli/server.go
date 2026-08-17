package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/server"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/spf13/cobra"
)

type Server struct {
	services.Config
}

func (s *Server) Customize(cmd *cobra.Command) {
	cmd.Hidden = true
}

func (s *Server) Run(cmd *cobra.Command, _ []string) error {
	// Replaces the interactive handler the root command installed. The
	// bridges must follow: they capture the handler installed here.
	logger.Setup(logger.JSON)
	services.SetupLogBridges()

	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer cancel()

	return server.Run(ctx, s.Config)
}
