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
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenUsageTimePeriod = 24 * time.Hour

var (
	openAIBaseURL    = "https://api.openai.com/v1"
	anthropicBaseURL = "https://api.anthropic.com/v1"
)

func init() {
	if base := os.Getenv("OPENAI_BASE_URL"); base != "" {
		openAIBaseURL = base
	}
	if base := os.Getenv("ANTHROPIC_BASE_URL"); base != "" {
		anthropicBaseURL = base
	}
}

func (s *Server) dispatchLLMProxy(req api.Context) error {
	token, err := s.tokenService.DecodeToken(req.Context(), strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))
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
	var modelID string
	if modelProvider == "" || modelStr != token.Model {
		// First, check that the user has token usage available for this request.
		if token.UserID != "" {
			remainingUsage, err := req.GatewayClient.RemainingTokenUsageForUser(req.Context(), token.UserID, tokenUsageTimePeriod, s.dailyUserTokenPromptTokenLimit, s.dailyUserTokenCompletionTokenLimit)
			if err != nil {
				return err
			} else if !remainingUsage.UnlimitedPromptTokens && remainingUsage.PromptTokens <= 0 || !remainingUsage.UnlimitedCompletionTokens && remainingUsage.CompletionTokens <= 0 {
				return types2.NewErrHTTP(http.StatusTooManyRequests, fmt.Sprintf("no tokens remaining (prompt tokens remaining: %d, completion tokens remaining: %d)", remainingUsage.PromptTokens, remainingUsage.CompletionTokens))
			}
		}

		m, err := getModelFromReference(req.Context(), req.Storage, token.Namespace, modelStr)
		if err != nil {
			return fmt.Errorf("failed to get model: %w", err)
		}

		modelID = m.Name
		modelProvider = m.Spec.Manifest.ModelProvider
		model = m.Spec.Manifest.TargetModel
	} else {
		// If this request is using a user-specific credential, then get it.
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", strings.Replace(token.TopLevelProjectID, system.ThreadPrefix, system.ProjectPrefix, 1), token.ModelProvider)}, token.ModelProvider)
		if err != nil {
			return fmt.Errorf("model provider not configured, failed to get credential: %w", err)
		}

		credEnv = cred.Env
		personalToken = true
	}

	// Check if the user has permission to use this model
	if modelID != "" && token.UserID != "" {
		userID, err := strconv.ParseUint(token.UserID, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse user ID: %w", err)
		}

		// Get the user's auth provider groups
		authProviderGroups, err := req.GatewayClient.ListGroupIDsForUser(req.Context(), uint(userID))
		if err != nil {
			return fmt.Errorf("failed to get user groups: %w", err)
		}

		hasAccess, err := s.mapHelper.UserHasAccessToModel(&user.DefaultInfo{
			UID:    token.UserID,
			Groups: token.UserGroups,
			Extra: map[string][]string{
				"auth_provider_groups": authProviderGroups,
			},
		}, modelID)
		if err != nil {
			return fmt.Errorf("failed to check model permission: %w", err)
		}
		if !hasAccess {
			return types2.NewErrForbidden("user does not have permission to use model %q (%s)", model, modelID)
		}
	}

	body["model"] = model
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req.Request.Body = io.NopCloser(bytes.NewReader(b))
	req.ContentLength = int64(len(b))

	u, err := s.dispatcher.URLForModelProvider(req.Context(), req.GPTClient, token.Namespace, modelProvider)
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
		Director:       dispatcher.TransformRequest(u, credEnv),
		ModifyResponse: (&responseModifier{userID: token.UserID, runID: token.RunID, client: req.GatewayClient, personalToken: personalToken}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

// getModelFromReference retrieves the model with a matching reference name.
// The reference name must be any one of the following:
// - The target name of a default model alias
// - The target name of the model itself
// - The actual name of the model
func getModelFromReference(ctx context.Context, client kclient.Client, namespace, modelReference string) (*v1.Model, error) {
	m, err := alias.GetFromScope(ctx, client, "Model", namespace, modelReference)
	if apierrors.IsNotFound(err) {
		// Maybe the user is trying to get a model by the target name.
		var models v1.ModelList
		if err := client.List(ctx, &models, &kclient.ListOptions{
			Namespace:     namespace,
			FieldSelector: fields.OneTermEqualSelector("spec.manifest.targetModel", modelReference),
		}); err != nil {
			return nil, err
		}

		if len(models.Items) == 0 {
			// Return the original error if no models are found.
			return nil, err
		}

		// Return the oldest one.
		sort.Slice(models.Items, func(i, j int) bool {
			return models.Items[i].CreationTimestamp.Before(&models.Items[j].CreationTimestamp)
		})

		return &models.Items[0], nil
	} else if err != nil {
		return nil, err
	}

	var respModel *v1.Model
	switch m := m.(type) {
	case *v1.DefaultModelAlias:
		if m.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", modelReference)
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

	return nil, fmt.Errorf("model %q not found", modelReference)
}

func envVarForModelProvider(modelProvider v1.ToolReference) (string, error) {
	if modelProvider.Status.Tool == nil {
		return "", fmt.Errorf("model provider %q is not configured", modelProvider.Name)
	}

	var providerMeta struct {
		EnvVars []types2.ProviderConfigurationParameter
	}

	if err := json.Unmarshal([]byte(modelProvider.Status.Tool.Metadata["providerMeta"]), &providerMeta); err != nil {
		return "", fmt.Errorf("failed to unmarshal model provider metadata: %w", err)
	}

	for _, envVar := range providerMeta.EnvVars {
		if strings.HasSuffix(envVar.Name, "_MODEL_PROVIDER_API_KEY") {
			return envVar.Name, nil
		}
	}

	return "", fmt.Errorf("model provider %q does not have an API key", modelProvider.Name)
}

func readBody(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return nil, err
	}

	return m, nil
}

// copyBody returns a copy of the bytes in a request body.
// If the copy was successful the request body is restored to its original state before returning so that
// it can be reused.
// The returned byte slice is safe to modify without affecting the request body.
func copyBody(r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(r.Body)
	r.Body.Close()

	if err != nil {
		// b can be partial results on error, don't restore the body
		return nil, err
	}

	// Read was successful, restore the body with a copy.
	r.Body = io.NopCloser(bytes.NewReader(slices.Clone(b)))
	return b, nil
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
	promptTokens += usage.Get("input_tokens").Int()
	completionTokens := usage.Get("completion_tokens").Int()
	completionTokens += usage.Get("output_tokens").Int()
	totalTokens := usage.Get("total_tokens").Int()

	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}

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

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

type llmProviderProxy struct {
	dailyUserTokenPromptTokenLimit     int
	dailyUserTokenCompletionTokenLimit int
	u                                  url.URL
	modelProviderName                  string
	modelProvider                      *v1.ToolReference
	mapHelper                          *modelaccesspolicy.Helper
	lock                               sync.RWMutex
}

func (s *Server) newLLMProviderProxy(u *url.URL, modelProviderName string) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserTokenPromptTokenLimit:     s.dailyUserTokenPromptTokenLimit,
		dailyUserTokenCompletionTokenLimit: s.dailyUserTokenCompletionTokenLimit,
		u:                                  *u,
		modelProviderName:                  modelProviderName,
		mapHelper:                          s.mapHelper,
	}
}

func (l *llmProviderProxy) proxy(req api.Context) error {
	l.lock.RLock()
	modelProvider := l.modelProvider
	l.lock.RUnlock()

	if modelProvider == nil {
		modelProvider = new(v1.ToolReference)
		if err := req.Get(modelProvider, l.modelProviderName); err != nil {
			return fmt.Errorf("model provider %s not found: %w", l.modelProviderName, err)
		}

		l.lock.Lock()
		l.modelProvider = modelProvider
		l.lock.Unlock()
	}

	// Attempt to get the target model
	body, err := copyBody(req.Request)
	if err != nil {
		return fmt.Errorf("failed to copy body: %w", err)
	}

	if targetModel := gjson.GetBytes(body, "model").String(); targetModel != "" {
		// Get the models matching the target model and provider.
		var models v1.ModelList
		if err := req.List(&models, &kclient.ListOptions{
			Namespace: l.modelProvider.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.manifest.targetModel":   targetModel,
				"spec.manifest.modelProvider": l.modelProvider.Name,
			}),
		}); err != nil {
			return fmt.Errorf("failed to list models: %w", err)
		}

		var hasAccess bool
		for _, model := range models.Items {
			var err error
			hasAccess, err = l.mapHelper.UserHasAccessToModel(req.User, model.Name)
			if err != nil {
				return fmt.Errorf("failed to check user access to model %q: %w", model.Name, err)
			}
			if hasAccess {
				break
			}
		}

		if !hasAccess {
			return types2.NewErrForbidden("user does not have permission to use model %q", targetModel)
		}
	}

	remainingUsage, err := req.GatewayClient.RemainingTokenUsageForUser(req.Context(), req.User.GetUID(), tokenUsageTimePeriod, l.dailyUserTokenPromptTokenLimit, l.dailyUserTokenCompletionTokenLimit)
	if err != nil {
		return err
	} else if !remainingUsage.UnlimitedPromptTokens && remainingUsage.PromptTokens <= 0 || !remainingUsage.UnlimitedCompletionTokens && remainingUsage.CompletionTokens <= 0 {
		return types2.NewErrHTTP(http.StatusTooManyRequests, fmt.Sprintf("no tokens remaining (prompt tokens remaining: %d, completion tokens remaining: %d)", remainingUsage.PromptTokens, remainingUsage.CompletionTokens))
	}

	credEnv, err := dispatcher.CredentialEnvForModelProvider(req.Context(), req.GPTClient, *modelProvider)
	if err != nil {
		return fmt.Errorf("failed to get credential environment for model provider: %w", err)
	}

	credEnvKey, err := envVarForModelProvider(*modelProvider)
	if err != nil {
		return fmt.Errorf("failed to get credential environment key for model provider: %w", err)
	}

	if bearer := req.Request.Header.Get("Authorization"); bearer != "" {
		req.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", credEnv[credEnvKey]))
	} else if token := req.Request.Header.Get("X-Api-Key"); token != "" {
		req.Request.Header.Set("X-Api-Key", credEnv[credEnvKey])
	}

	(&httputil.ReverseProxy{
		Director:       dispatcher.TransformRequest(l.u, nil),
		ModifyResponse: (&responseModifier{userID: req.User.GetUID(), client: req.GatewayClient}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}
