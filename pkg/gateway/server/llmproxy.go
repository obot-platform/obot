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
	"github.com/obot-platform/obot/pkg/messagepolicy"
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
		Director: llmTransformRequest(u, credEnv),
		ModifyResponse: (&responseModifier{
			userID:        token.UserID,
			runID:         token.RunID,
			model:         model,
			client:        req.GatewayClient,
			personalToken: personalToken,
		}).modifyResponse,
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

// extractModelFromBody extracts the model name from an arbitrary JSON payload or body,
// checking top-level "model" first, then nested paths such as "message.model" and "response.model"
// used by different providers or event/response shapes.
func extractModelFromBody(body []byte) string {
	if model := gjson.GetBytes(body, "model").String(); model != "" {
		return model
	}
	if model := gjson.GetBytes(body, "message.model").String(); model != "" {
		return model
	}
	return gjson.GetBytes(body, "response.model").String()
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
	userID, runID, model                        string
	personalToken                               bool
	client                                      *client.Client
	lock                                        sync.Mutex
	promptTokens, completionTokens, totalTokens int
	b                                           *bufio.Reader
	c                                           io.Closer
	stream                                      bool
	leftover                                    []byte

	// Output (tool-call) policy evaluation fields.
	messagePolicyHelper *messagepolicy.Helper
	outputPolicies      []types2.MessagePolicyManifest
	conversationHistory []messagepolicy.ConversationMessage
	pipeReader          *io.PipeReader // set when output policies are active; Read() reads from this
}

func (r *responseModifier) modifyResponse(resp *http.Response) error {
	if resp.StatusCode != http.StatusOK || (resp.Request.URL.Path != "/v1/chat/completions" && resp.Request.URL.Path != "/v1/messages" && resp.Request.URL.Path != "/v1/responses") {
		return nil
	}

	r.c = resp.Body
	r.b = bufio.NewReader(resp.Body)
	r.stream = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	resp.Body = r

	// When tool-call output policies are active, launch a goroutine that streams text through
	// while buffering tool call chunks for policy evaluation.
	if len(r.outputPolicies) > 0 {
		pr, pw := io.Pipe()
		r.pipeReader = pr
		go r.streamAndEvaluateToolCalls(pw)
	}

	return nil
}

func (r *responseModifier) Read(p []byte) (int, error) {
	// When output policies are active, the goroutine handles everything via the pipe.
	if r.pipeReader != nil {
		return r.pipeReader.Read(p)
	}

	if len(r.leftover) > 0 {
		n := copy(p, r.leftover)
		r.leftover = r.leftover[n:]
		return n, nil
	}

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

	r.extractTokenUsage(line)

	n := copy(p, prefix)
	if n < len(prefix) {
		// We couldn't copy the entire prefix, so save the leftover for the next read and return.
		r.leftover = append(prefix[n:], line...)
		return n, nil
	}

	n += copy(p[n:], line)

	// If we didn't copy the entire line, then save the leftover for the next read.
	r.leftover = line[n-len(prefix):]

	return n, nil
}

// extractTokenUsage extracts token counts from a JSON line (with data: prefix already stripped).
func (r *responseModifier) extractTokenUsage(line []byte) {
	// Extract token usage from all known locations.
	// Providers nest usage differently:
	//   - OpenAI Chat Completions: top-level "usage"
	//   - Anthropic message_start: "message.usage"
	//   - Anthropic message_delta: top-level "usage" (cumulative)
	//   - OpenAI Responses API:    "response.usage"
	// Use max to handle cumulative token counts (e.g. Anthropic's message_delta
	// reports cumulative output_tokens, not incremental).
	usage := gjson.GetBytes(line, "usage")
	promptTokens := max(usage.Get("prompt_tokens").Int(), usage.Get("input_tokens").Int())
	completionTokens := max(usage.Get("completion_tokens").Int(), usage.Get("output_tokens").Int())
	totalTokens := usage.Get("total_tokens").Int()

	if msgUsage := gjson.GetBytes(line, "message.usage"); msgUsage.Exists() {
		promptTokens = max(promptTokens, msgUsage.Get("input_tokens").Int())
		completionTokens = max(completionTokens, msgUsage.Get("output_tokens").Int())
	}

	if respUsage := gjson.GetBytes(line, "response.usage"); respUsage.Exists() {
		promptTokens = max(promptTokens, respUsage.Get("input_tokens").Int())
		completionTokens = max(completionTokens, respUsage.Get("output_tokens").Int())
		totalTokens = max(totalTokens, respUsage.Get("total_tokens").Int())
	}

	if promptTokens > 0 || completionTokens > 0 || totalTokens > 0 {
		r.lock.Lock()
		r.promptTokens = max(r.promptTokens, int(promptTokens))
		r.completionTokens = max(r.completionTokens, int(completionTokens))
		r.totalTokens = max(r.totalTokens, int(totalTokens))
		r.lock.Unlock()
	}
}

// streamAndEvaluateToolCalls reads the upstream response, streams text through immediately,
// buffers tool call chunks, and evaluates policies only against tool calls.
// For non-streaming responses, it buffers the entire response and evaluates inline.
func (r *responseModifier) streamAndEvaluateToolCalls(pw *io.PipeWriter) {
	defer pw.Close()

	logger.Debugf("Output tool-call policies active for user=%s, streaming text while collecting tool calls", r.userID)

	if r.stream {
		r.streamAndEvaluateToolCallsSSE(pw)
	} else {
		r.streamAndEvaluateToolCallsJSON(pw)
	}
}

// streamAndEvaluateToolCallsSSE handles streaming (SSE) responses.
// Text delta chunks are forwarded immediately; tool call chunks are buffered for evaluation.
// Write errors on pw are intentionally discarded: the pipe reader will surface any errors,
// and a broken pipe simply means the client disconnected.
func (r *responseModifier) streamAndEvaluateToolCallsSSE(pw *io.PipeWriter) {
	var (
		toolCallChunks [][]byte // raw SSE lines containing tool_call deltas (held back)
		toolCalls      []messagepolicy.ToolCallInfo
		finishLine     []byte // the line containing finish_reason (held until evaluation)
	)

	for {
		line, err := r.b.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			break
		}

		rest, isData := bytes.CutPrefix(line, []byte("data: "))
		if !isData {
			// Non-data lines (empty lines, event: lines) — pass through.
			_, _ = pw.Write(line)
			if err != nil {
				break
			}
			continue
		}

		// Extract token usage from every data line.
		r.extractTokenUsage(rest)

		// Check if this chunk contains tool_calls or a finish_reason.
		delta := gjson.GetBytes(rest, "choices.0.delta")
		hasToolCalls := delta.Get("tool_calls").Exists()
		finishReason := gjson.GetBytes(rest, "choices.0.finish_reason")

		if hasToolCalls {
			// Buffer tool call chunks — do not send to client yet.
			toolCallChunks = append(toolCallChunks, append([]byte(nil), line...))

			// Accumulate tool call info for policy evaluation.
			delta.Get("tool_calls").ForEach(func(_, tc gjson.Result) bool {
				name := tc.Get("function.name").String()
				args := tc.Get("function.arguments").String()
				idx := int(tc.Get("index").Int())

				for len(toolCalls) <= idx {
					toolCalls = append(toolCalls, messagepolicy.ToolCallInfo{})
				}
				if name != "" {
					toolCalls[idx].Name += name
				}
				if args != "" {
					toolCalls[idx].Arguments += args
				}
				return true
			})
		} else if finishReason.Exists() && finishReason.Type != gjson.Null {
			// Hold the finish line until we decide whether to forward or replace.
			finishLine = append([]byte(nil), line...)
		} else {
			// Text content or other non-tool-call data — forward immediately.
			_, _ = pw.Write(line)
		}

		if err != nil {
			break
		}
	}

	logger.Debugf("Response complete for user=%s: %d tool calls detected", r.userID, len(toolCalls))

	if len(toolCalls) == 0 {
		// No tool calls — forward the finish line and done.
		logger.Debugf("No tool calls in response for user=%s, skipping policy evaluation", r.userID)
		if len(finishLine) > 0 {
			_, _ = pw.Write(finishLine)
		}
		return
	}

	// Evaluate policies against tool calls only.
	targetMessage := buildToolCallTargetMessage(toolCalls)
	logger.Debugf("Evaluating %d tool-call policies against %d tool calls for user=%s", len(r.outputPolicies), len(toolCalls), r.userID)
	violations := r.messagePolicyHelper.EvaluateMessage(context.Background(), r.outputPolicies, r.conversationHistory, targetMessage, types2.PolicyDirectionToolCalls)

	if len(violations) == 0 {
		// No violations — flush all buffered tool call chunks + finish line.
		logger.Debugf("Tool call policy evaluation passed for user=%s, forwarding %d tool calls", r.userID, len(toolCalls))
		for _, chunk := range toolCallChunks {
			_, _ = pw.Write(chunk)
		}
		if len(finishLine) > 0 {
			_, _ = pw.Write(finishLine)
		}
		return
	}

	// Violation — suppress tool calls, emit notification.
	logger.Infof("Tool call policy violation detected for user=%s, suppressing %d tool calls", r.userID, len(toolCalls))
	var explanations []string
	for _, v := range violations {
		explanations = append(explanations, v.Explanation)
	}

	notification := fmt.Sprintf(
		"<system_notification>Your tool call(s) were blocked by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: %s</system_notification>",
		strings.Join(explanations, "\n"),
	)

	_, _ = pw.Write(buildStreamingNotificationChunks(notification, r.model))
}

// streamAndEvaluateToolCallsJSON handles non-streaming (single JSON) responses.
func (r *responseModifier) streamAndEvaluateToolCallsJSON(pw *io.PipeWriter) {
	// Read the entire response body. Error is discarded because the upstream
	// response may legitimately EOF, and partial reads are handled gracefully.
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r.b)
	body := buf.Bytes()

	r.extractTokenUsage(body)

	// Check if the response contains tool calls.
	tcResult := gjson.GetBytes(body, "choices.0.message.tool_calls")
	if !tcResult.Exists() || len(tcResult.Array()) == 0 {
		// No tool calls — pass through unchanged.
		logger.Debugf("No tool calls in response for user=%s, skipping policy evaluation", r.userID)
		_, _ = pw.Write(body)
		return
	}

	// Extract tool call info for policy evaluation.
	var toolCalls []messagepolicy.ToolCallInfo
	tcResult.ForEach(func(_, tc gjson.Result) bool {
		toolCalls = append(toolCalls, messagepolicy.ToolCallInfo{
			Name:      tc.Get("function.name").String(),
			Arguments: tc.Get("function.arguments").String(),
		})
		return true
	})

	targetMessage := buildToolCallTargetMessage(toolCalls)
	logger.Debugf("Evaluating %d tool-call policies against %d tool calls for user=%s", len(r.outputPolicies), len(toolCalls), r.userID)
	violations := r.messagePolicyHelper.EvaluateMessage(context.Background(), r.outputPolicies, r.conversationHistory, targetMessage, types2.PolicyDirectionToolCalls)

	if len(violations) == 0 {
		// No violations — pass through unchanged.
		logger.Debugf("Tool call policy evaluation passed for user=%s, forwarding %d tool calls", r.userID, len(toolCalls))
		_, _ = pw.Write(body)
		return
	}

	// Violation — build replacement response without tool calls.
	logger.Infof("Tool call policy violation detected for user=%s, suppressing %d tool calls", r.userID, len(toolCalls))
	var explanations []string
	for _, v := range violations {
		explanations = append(explanations, v.Explanation)
	}

	notification := fmt.Sprintf(
		"<system_notification>Your tool call(s) were blocked by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: %s</system_notification>",
		strings.Join(explanations, "\n"),
	)

	_, _ = pw.Write(buildReplacementResponse(notification, r.model))
}

// buildToolCallTargetMessage formats tool calls into the target message string for the policy judge.
func buildToolCallTargetMessage(toolCalls []messagepolicy.ToolCallInfo) string {
	var sb strings.Builder
	for i, tc := range toolCalls {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "[called tool %q with args: %s]", tc.Name, tc.Arguments)
	}
	return sb.String()
}

// buildStreamingNotificationChunks produces SSE chunks that send a text delta with the
// notification content, followed by a stop finish chunk and [DONE].
func buildStreamingNotificationChunks(notification, model string) []byte {
	textDelta, _ := json.Marshal(map[string]any{
		"id":     "policy-violation",
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{"content": notification},
				"finish_reason": nil,
			},
		},
	})

	stopDelta, _ := json.Marshal(map[string]any{
		"id":     "policy-violation",
		"object": "chat.completion.chunk",
		"model":  model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	})

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "data: %s\n\n", textDelta)
	fmt.Fprintf(&buf, "data: %s\n\n", stopDelta)
	buf.WriteString("data: [DONE]\n\n")
	return buf.Bytes()
}

// buildReplacementResponse constructs a minimal OpenAI-format chat completion response
// with the given content replacing the assistant's message (no tool calls).
func buildReplacementResponse(content, model string) []byte {
	resp := map[string]any{
		"id":     "policy-violation",
		"object": "chat.completion",
		"model":  model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}

	b, _ := json.Marshal(resp)
	return append(b, '\n')
}

func (r *responseModifier) Close() error {
	r.lock.Lock()
	totalTokens := r.totalTokens
	if totalTokens == 0 {
		totalTokens = r.promptTokens + r.completionTokens
	}
	activity := &types.RunTokenActivity{
		Name:             r.runID,
		UserID:           r.userID,
		Model:            r.model,
		PromptTokens:     r.promptTokens,
		CompletionTokens: r.completionTokens,
		TotalTokens:      totalTokens,
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

func llmTransformRequest(u url.URL, credEnv map[string]string) func(req *http.Request) {
	transform := dispatcher.TransformRequest(u, credEnv)

	return func(req *http.Request) {
		transform(req)

		// Ensure the upstream transport can transparently decode compressed responses.
		// If we forward Accept-Encoding from the client, net/http transport will not
		// auto-decompress and token usage parsing can see compressed bytes.
		req.Header.Del("Accept-Encoding")
	}
}

// parseMessagesFromBody converts the raw messages array from the request body into
// ConversationMessages for policy evaluation. Returns the conversation history,
// the last user message content, and its index in the raw array (-1 if not found).
func parseMessagesFromBody(rawMessages []any) ([]messagepolicy.ConversationMessage, string, int) {
	var (
		history     []messagepolicy.ConversationMessage
		lastUserMsg string
		lastUserIdx = -1
	)

	for i, raw := range rawMessages {
		msg, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		role, _ := msg["role"].(string)
		content := extractContentString(msg["content"])

		cm := messagepolicy.ConversationMessage{
			Role:    role,
			Content: content,
		}

		// Parse tool_call_id for tool messages.
		if toolCallID, ok := msg["tool_call_id"].(string); ok {
			cm.ToolCallID = toolCallID
		}

		// Parse tool_calls for assistant messages.
		if toolCalls, ok := msg["tool_calls"].([]any); ok {
			for _, tc := range toolCalls {
				tcMap, ok := tc.(map[string]any)
				if !ok {
					continue
				}
				fn, _ := tcMap["function"].(map[string]any)
				if fn == nil {
					continue
				}
				name, _ := fn["name"].(string)
				arguments, _ := fn["arguments"].(string)
				cm.ToolCalls = append(cm.ToolCalls, messagepolicy.ToolCallInfo{
					Name:      name,
					Arguments: arguments,
				})
			}
		}

		history = append(history, cm)

		if role == "user" {
			lastUserMsg = content
			lastUserIdx = i
		}
	}

	return history, lastUserMsg, lastUserIdx
}

// extractContentString extracts a text string from the OpenAI message content field,
// which can be either a plain string or an array of content parts.
func extractContentString(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		// Array of content parts — extract text parts.
		var parts []string
		for _, part := range c {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if partMap["type"] == "text" {
				if text, ok := partMap["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

type llmProviderProxy struct {
	dailyUserTokenPromptTokenLimit     int
	dailyUserTokenCompletionTokenLimit int
	u                                  url.URL
	modelProviderName                  string
	modelProvider                      *v1.ToolReference
	mapHelper                          *modelaccesspolicy.Helper
	messagePolicyHelper                *messagepolicy.Helper
	lock                               sync.RWMutex
}

func (s *Server) newLLMProviderProxy(u *url.URL, modelProviderName string) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserTokenPromptTokenLimit:     s.dailyUserTokenPromptTokenLimit,
		dailyUserTokenCompletionTokenLimit: s.dailyUserTokenCompletionTokenLimit,
		u:                                  *u,
		modelProviderName:                  modelProviderName,
		mapHelper:                          s.mapHelper,
		messagePolicyHelper:                s.messagePolicyHelper,
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

	targetModel := extractModelFromBody(body)
	if targetModel != "" {
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

	// Evaluate message policies if the helper is available and we have a user.
	var (
		outputPolicies      []types2.MessagePolicyManifest
		conversationHistory []messagepolicy.ConversationMessage
		bodyModified        bool
	)
	if l.messagePolicyHelper != nil && req.User.GetUID() != "" {
		var bodyMap map[string]any
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			// Evaluate user message against applicable input policies.
			inputPolicies, err := l.messagePolicyHelper.GetApplicablePolicies(req.User, types2.PolicyDirectionUserMessage)
			if err == nil && len(inputPolicies) > 0 {
				rawMessages, _ := bodyMap["messages"].([]any)
				if len(rawMessages) > 0 {
					history, lastUserMsg, lastUserIdx := parseMessagesFromBody(rawMessages)
					if lastUserIdx >= 0 {
						violations := l.messagePolicyHelper.EvaluateMessage(req.Context(), inputPolicies, history, lastUserMsg, types2.PolicyDirectionUserMessage)
						if len(violations) > 0 {
							var explanations []string
							for _, v := range violations {
								explanations = append(explanations, v.Explanation)
							}
							replacement := fmt.Sprintf(
								"<system_notification>This message was removed by the system for violating policies. Inform the user that you cannot complete their requested action due to a policy violation. The following explanation was generated for you to relay to the user: %s</system_notification>",
								strings.Join(explanations, "\n"),
							)
							if msgMap, ok := rawMessages[lastUserIdx].(map[string]any); ok {
								msgMap["content"] = replacement
								bodyModified = true
							}
						}
					}
				}
			} else if err != nil {
				return fmt.Errorf("failed to get applicable input policies: %w", err)
			}

			// Check for output (tool-call) policies.
			outputPolicies, _ = l.messagePolicyHelper.GetApplicablePolicies(req.User, types2.PolicyDirectionToolCalls)
			if len(outputPolicies) > 0 {
				rawMessages, _ := bodyMap["messages"].([]any)
				conversationHistory, _, _ = parseMessagesFromBody(rawMessages)
			}

			// Re-serialize the body if input policies modified it.
			if bodyModified {
				b, err := json.Marshal(bodyMap)
				if err != nil {
					return fmt.Errorf("failed to marshal modified body: %w", err)
				}
				req.Request.Body = io.NopCloser(bytes.NewReader(b))
				req.ContentLength = int64(len(b))
			}
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
		Director: llmTransformRequest(l.u, nil),
		ModifyResponse: (&responseModifier{
			userID:              req.User.GetUID(),
			model:               targetModel,
			client:              req.GatewayClient,
			messagePolicyHelper: l.messagePolicyHelper,
			outputPolicies:      outputPolicies,
			conversationHistory: conversationHistory,
		}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}
