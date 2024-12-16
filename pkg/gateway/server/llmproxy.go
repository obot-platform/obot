package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"

	types2 "github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/api"
	"github.com/acorn-io/acorn/pkg/gateway/types"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
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

	// Get the run so that we know what the namespace of the model provider is
	var run v1.Run
	if err = req.Get(&run, token.RunID); err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	errChan := make(chan error, 1)
	(&httputil.ReverseProxy{
		Director: s.newDirector(run.Namespace, errChan),
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return <-errChan
}

func (s *Server) newDirector(namespace string, errChan chan<- error) func(req *http.Request) {
	return func(req *http.Request) {
		errChan <- s.modelDispatcher.TransformRequest(req, namespace)
	}
}
