package server

import (
	"fmt"
)

func (s *Server) authCompleteURL() string {
	return fmt.Sprintf("%s/login_complete", s.uiURL)
}
