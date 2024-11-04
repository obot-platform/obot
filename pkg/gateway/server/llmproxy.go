package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"

	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
)

func (s *Server) llmProxy(req api.Context) error {
	token, err := s.tokenService.DecodeToken(strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return types2.NewErrHttp(http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err))
	}

	if err = s.db.WithContext(req.Context()).Create(&types.LLMProxyActivity{
		WorkflowID:     token.WorkflowID,
		WorkflowStepID: token.WorkflowStepID,
		AgentID:        token.AgentID,
		ThreadID:       token.ThreadID,
		RunID:          token.RunID,
		Path:           req.URL.Path,
	}).Error; err != nil {
		return fmt.Errorf("failed to create monitor: %w", err)
	}

	errChan := make(chan error, 1)
	(&httputil.ReverseProxy{
		Director: s.newDirector(errChan),
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return <-errChan
}

func (s *Server) newDirector(errChan chan<- error) func(req *http.Request) {
	return func(req *http.Request) {
		errChan <- s.modelDispatcher.TransformRequest(req)
	}
}
