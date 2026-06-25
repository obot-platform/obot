package server

func (s *Server) authCompleteURL() string {
	return s.uiURL + "/auth/oauth/complete"
}
