package cli

import (
	"github.com/acorn-io/acorn/pkg/server"
	"github.com/acorn-io/acorn/pkg/services"
	"github.com/spf13/cobra"
)

type Server struct {
	services.Config
}

func (s *Server) Run(cmd *cobra.Command, _ []string) error {
	return server.Run(cmd.Context(), s.Config)
}
