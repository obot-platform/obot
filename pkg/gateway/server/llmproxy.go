package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/tidwall/gjson"
)

const tokenUsageTimePeriod = 24 * time.Hour

func (s *Server) llmProxy(req api.Context) error {
	token, err := s.tokenService.DecodeToken(strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return types2.NewErrHTTP(http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err))
	}

	if token.UserID != "" {
		remainingUsage, err := s.client.RemainingTokenUsageForUser(req.Context(), token.UserID, tokenUsageTimePeriod, s.dailyUserTokenPromptTokenLimit, s.dailyUserTokenCompletionTokenLimit)
		if err != nil {
			return err
		} else if !remainingUsage.UnlimitedPromptTokens && remainingUsage.PromptTokens <= 0 || !remainingUsage.UnlimitedCompletionTokens && remainingUsage.CompletionTokens <= 0 {
			return types2.NewErrHTTP(http.StatusTooManyRequests, fmt.Sprintf("no tokens remaining (prompt tokens: %d, completion tokens: %d)", remainingUsage.PromptTokens, remainingUsage.CompletionTokens))
		}
	}

	if err = s.db.WithContext(req.Context()).Create(&types.LLMProxyActivity{
		UserID:         token.UserID,
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
		Director:       s.newDirector(token.Namespace, errChan),
		ModifyResponse: (&responseModifier{userID: token.UserID, runID: token.RunID, client: s.client}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return <-errChan
}

func (s *Server) newDirector(namespace string, errChan chan<- error) func(req *http.Request) {
	return func(req *http.Request) {
		errChan <- s.dispatcher.TransformRequest(req, namespace)
	}
}

type responseModifier struct {
	userID, runID                               string
	client                                      *client.Client
	lock                                        sync.Mutex
	promptTokens, completionTokens, totalTokens int
	b                                           *bufio.Reader
	c                                           io.Closer
	stream                                      bool
}

func (r *responseModifier) modifyResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/v1/chat/completions" {
		return nil
	}

	r.c = resp.Body
	r.b = bufio.NewReader(resp.Body)
	r.stream = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	resp.Body = r

	return nil
}

func (r *responseModifier) Read(p []byte) (int, error) {
	line, err := r.b.ReadBytes('\n')
	if len(line) > 0 && errors.Is(err, io.EOF) {
		// Don't send an EOF until we read everything.
		err = nil
	}
	if err != nil {
		return copy(p, line), err
	}

	var prefix []byte
	if r.stream {
		prefix = []byte("data: ")
		rest, ok := bytes.CutPrefix(line, prefix)
		if !ok {
			// This isn't a data line, so send it through.
			return copy(p, line), nil
		}
		line = rest
	}

	usage := gjson.GetBytes(line, "usage")
	promptTokens := usage.Get("prompt_tokens").Int()
	completionTokens := usage.Get("completion_tokens").Int()
	totalTokens := usage.Get("total_tokens").Int()

	if promptTokens > 0 || completionTokens > 0 || totalTokens > 0 {
		r.lock.Lock()
		r.promptTokens += int(promptTokens)
		r.completionTokens += int(completionTokens)
		r.totalTokens += int(totalTokens)
		r.lock.Unlock()
	}

	var n int
	if len(prefix) > 0 {
		n = copy(p, prefix)
	}

	n += copy(p[n:], line)
	return n, nil
}

func (r *responseModifier) Close() error {
	r.lock.Lock()
	activity := &types.RunTokenActivity{
		Name:             r.runID,
		UserID:           r.userID,
		PromptTokens:     r.promptTokens,
		CompletionTokens: r.completionTokens,
		TotalTokens:      r.totalTokens,
	}
	r.lock.Unlock()
	if err := r.client.InsertTokenUsage(context.Background(), activity); err != nil {
		logger.Warnf("failed to save token usage for run %s: %v", r.runID, err)
	}
	return r.c.Close()
}
