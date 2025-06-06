package cli

import (
	"github.com/obot-platform/obot/pkg/server"
	"github.com/obot-platform/obot/pkg/services"
	"github.com/spf13/cobra"
)

type Server struct {
	services.Config
}

func (s *Server) Run(cmd *cobra.Command, _ []string) error {
	return server.Run(cmd.Context(), s.Config)
}
