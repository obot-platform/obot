package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenUsageTimePeriod = 24 * time.Hour

func (s *Server) llmProxy(req api.Context) error {
	token, err := s.tokenService.DecodeToken(strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		return types2.NewErrHTTP(http.StatusUnauthorized, fmt.Sprintf("invalid token: %v", err))
	}

	var (
		credEnv       map[string]string
		personalToken bool
		model         = token.Model
		modelProvider = token.ModelProvider
	)

	body, err := readBody(req.Request)
	if err != nil {
		return fmt.Errorf("failed to read body: %w", err)
	}

	modelStr, ok := body["model"].(string)
	if !ok {
		return fmt.Errorf("missing model in body")
	}

	// If the model string is different from the model, then we need to look up the model in our database to get the
	// correct model and model provider information.
	if modelProvider == "" || modelStr != token.Model {
		// First, check that the user has token usage available for this request.
		if token.UserID != "" {
			remainingUsage, err := s.client.RemainingTokenUsageForUser(req.Context(), token.UserID, tokenUsageTimePeriod, s.dailyUserTokenPromptTokenLimit, s.dailyUserTokenCompletionTokenLimit)
			if err != nil {
				return err
			} else if !remainingUsage.UnlimitedPromptTokens && remainingUsage.PromptTokens <= 0 || !remainingUsage.UnlimitedCompletionTokens && remainingUsage.CompletionTokens <= 0 {
				return types2.NewErrHTTP(http.StatusTooManyRequests, fmt.Sprintf("no tokens remaining (prompt tokens: %d, completion tokens: %d)", remainingUsage.PromptTokens, remainingUsage.CompletionTokens))
			}
		}

		m, err := getModelProviderForModel(req.Context(), s.storageClient, token.Namespace, modelStr)
		if err != nil {
			return fmt.Errorf("failed to get model: %w", err)
		}

		modelProvider = m.Spec.Manifest.ModelProvider
		model = m.Spec.Manifest.TargetModel
	} else {
		// If this request is using a user-specific credential, then get it.
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", strings.Replace(token.ProjectID, system.ThreadPrefix, system.ProjectPrefix, 1), token.ModelProvider)}, token.ModelProvider)
		if err != nil {
			return fmt.Errorf("model provider not configured, failed to get credential: %w", err)
		}

		credEnv = cred.Env
		personalToken = true
	}

	body["model"] = model
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req.Request.Body = io.NopCloser(bytes.NewReader(b))
	req.ContentLength = int64(len(b))

	u, err := s.dispatcher.URLForModelProvider(req.Context(), token.Namespace, modelProvider)
	if err != nil {
		return fmt.Errorf("failed to get model provider: %w", err)
	}

	if err = s.db.WithContext(req.Context()).Create(&types.LLMProxyActivity{
		UserID:         token.UserID,
		WorkflowID:     token.WorkflowID,
		WorkflowStepID: token.WorkflowStepID,
		AgentID:        token.AgentID,
		ProjectID:      token.ProjectID,
		ThreadID:       token.ThreadID,
		RunID:          token.RunID,
		Path:           req.URL.Path,
	}).Error; err != nil {
		return fmt.Errorf("failed to create monitor: %w", err)
	}

	(&httputil.ReverseProxy{
		Director:       s.dispatcher.TransformRequest(u, credEnv),
		ModifyResponse: (&responseModifier{userID: token.UserID, runID: token.RunID, client: s.client, personalToken: personalToken}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

func getModelProviderForModel(ctx context.Context, client kclient.Client, namespace, model string) (*v1.Model, error) {
	m, err := alias.GetFromScope(ctx, client, "Model", namespace, model)
	if err != nil {
		return nil, err
	}

	var respModel *v1.Model
	switch m := m.(type) {
	case *v1.DefaultModelAlias:
		if m.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", model)
		}
		var model v1.Model
		if err := alias.Get(ctx, client, &model, namespace, m.Spec.Manifest.Model); err != nil {
			return nil, err
		}
		respModel = &model
	case *v1.Model:
		respModel = m
	}

	if respModel != nil {
		if !respModel.Spec.Manifest.Active {
			return nil, fmt.Errorf("model %q is not active", respModel.Spec.Manifest.Name)
		}

		return respModel, nil
	}

	return nil, fmt.Errorf("model %q not found", model)
}

func readBody(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}

	return m, nil
}

type responseModifier struct {
	userID, runID                               string
	personalToken                               bool
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
		PersonalToken:    r.personalToken,
	}
	r.lock.Unlock()
	if err := r.client.InsertTokenUsage(context.Background(), activity); err != nil {
		logger.Warnf("failed to save token usage for run %s: %v", r.runID, err)
	}
	return r.c.Close()
}
