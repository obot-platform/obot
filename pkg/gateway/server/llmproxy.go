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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const tokenUsageTimePeriod = 24 * time.Hour

var (
	openAIBaseURL    = "https://api.openai.com/v1"
	anthropicBaseURL = "https://api.anthropic.com/v1"
)

const (
	internalRequestTypeHeader = "X-Nanobot-Internal-Request-Type"
	threadTitleRequestType    = "nanobot.summary.thread_title"
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

	// Fetch auth provider groups once for all checks that need them.
	var authProviderGroups []string
	if token.UserID != "" {
		userIDInt, err := strconv.ParseUint(token.UserID, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse user ID: %w", err)
		}
		authProviderGroups, err = req.GatewayClient.ListGroupIDsForUser(req.Context(), uint(userIDInt))
		if err != nil {
			return fmt.Errorf("failed to get user groups: %w", err)
		}
	}

	// Check if the user has permission to use this model
	if modelID != "" && token.UserID != "" {
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

	// Evaluate message policies.
	var (
		outputPolicies         []messagepolicy.ApplicablePolicy
		conversationHistory    []messagepolicy.ConversationMessage
		inputPolicyReplacement string
	)
	messagePolicyHelper := s.messagePolicyHelper
	if shouldSkipMessagePolicyEnforcement(req.Request) {
		messagePolicyHelper = nil
	}
	if messagePolicyHelper != nil && token.UserID != "" {
		userInfo := &user.DefaultInfo{
			UID:    token.UserID,
			Groups: token.UserGroups,
			Extra: map[string][]string{
				"auth_provider_groups": authProviderGroups,
			},
		}
		outputPolicies, conversationHistory, inputPolicyReplacement, err = applyMessagePolicies(
			req.Context(), messagePolicyHelper, userInfo, req.GatewayClient, body, token.ProjectID, token.ThreadID,
		)
		if err != nil {
			return err
		}
	}

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
			userID:                 token.UserID,
			runID:                  token.RunID,
			model:                  model,
			client:                 req.GatewayClient,
			personalToken:          personalToken,
			projectID:              token.ProjectID,
			threadID:               token.ThreadID,
			inputPolicyReplacement: inputPolicyReplacement,
			messagePolicyHelper:    messagePolicyHelper,
			outputPolicies:         outputPolicies,
			conversationHistory:    conversationHistory,
		}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

// getModelFromReference retrieves the model with a matching reference name.
// The reference name must be any one of the following:
// - The name of a default model alias
// - The actual Kubernetes resource name of the model
func getModelFromReference(ctx context.Context, client kclient.Client, namespace, modelReference string) (*v1.Model, error) {
	m, err := alias.GetFromScope(ctx, client, "Model", namespace, modelReference)
	if err != nil {
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

	return nil, apierrors.NewNotFound(schema.GroupResource{Group: v1.SchemeGroupVersion.Group, Resource: "model"}, modelReference)
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

func rewriteModelInBody(body []byte, model string) ([]byte, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, err
	}
	if _, ok := bodyMap["model"]; ok {
		bodyMap["model"] = model
	}
	return json.Marshal(bodyMap)
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
	projectID, threadID                         string
	personalToken                               bool
	client                                      *client.Client
	lock                                        sync.Mutex
	promptTokens, completionTokens, totalTokens int
	b                                           *bufio.Reader
	c                                           io.Closer
	stream                                      bool
	leftover                                    []byte

	// Input policy violation: replacement text to send back via response header.
	inputPolicyReplacement string

	// Output (tool-call) policy evaluation fields.
	messagePolicyHelper *messagepolicy.Helper
	outputPolicies      []messagepolicy.ApplicablePolicy
	conversationHistory []messagepolicy.ConversationMessage
	pipeReader          *io.PipeReader // set when output policies are active; Read() reads from this
}

func (r *responseModifier) modifyResponse(resp *http.Response) error {
	if r.inputPolicyReplacement != "" {
		resp.Header.Set("X-Obot-Message-Policy-Replacement", r.inputPolicyReplacement)
	}

	if resp.StatusCode != http.StatusOK || (resp.Request.URL.Path != "/v1/messages" && resp.Request.URL.Path != "/v1/responses") {
		return nil
	}

	r.c = resp.Body
	r.b = bufio.NewReader(resp.Body)
	r.stream = strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	resp.Body = r

	// When tool-call output policies are active, launch a goroutine that streams text
	// through immediately while buffering tool call chunks for policy evaluation.
	if len(r.outputPolicies) > 0 {
		pr, pw := io.Pipe()
		r.pipeReader = pr
		go r.streamAndEvaluateToolCalls(resp.Request.Context(), pw)
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
func (r *responseModifier) streamAndEvaluateToolCalls(ctx context.Context, pw *io.PipeWriter) {
	defer pw.Close()

	if r.stream {
		r.streamAndEvaluateToolCallsSSE(ctx, pw)
	} else {
		r.streamAndEvaluateToolCallsJSON(ctx, pw)
	}
}

// streamAndEvaluateToolCallsSSE handles streaming (SSE) responses.
// Text delta chunks are forwarded immediately; once the first tool_call chunk appears,
// all remaining lines are buffered until the stream ends. After policy evaluation,
// the buffered lines (including tool calls) are forwarded unmodified, and a violation
// marker is injected if a policy was violated. The downstream client (nanobot) detects
// this marker and returns error tool_results instead of executing the tools.
func (r *responseModifier) streamAndEvaluateToolCallsSSE(ctx context.Context, pw *io.PipeWriter) {
	var (
		buffered             [][]byte // all lines from the first tool_call onward, in original order
		toolCalls            []messagepolicy.ToolCallInfo
		seenToolCalls        bool
		anthropicBlockToTool map[int]int // maps Anthropic content block index → toolCalls slice index
		responsesItemToTool  map[int]int // maps Responses API output_index → toolCalls slice index
	)

	for {
		line, err := r.b.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			break
		}

		// Once we see the first tool_call chunk, buffer everything remaining
		// so we can evaluate policies before deciding what to send.
		if seenToolCalls {
			buffered = append(buffered, append([]byte(nil), line...))

			rest, isData := bytes.CutPrefix(line, []byte("data: "))
			if isData {
				r.extractTokenUsage(rest)

				// Anthropic format: content_block_start/content_block_delta
				accumulateAnthropicToolCallInfo(rest, &toolCalls, anthropicBlockToTool)
				// OpenAI Responses API format: response.output_item.added / response.function_call_arguments.delta
				accumulateResponsesAPIToolCallInfo(rest, &toolCalls, responsesItemToTool)
			}

			if err != nil {
				break
			}
			continue
		}

		rest, isData := bytes.CutPrefix(line, []byte("data: "))
		if !isData {
			_, _ = pw.Write(line)
			if err != nil {
				break
			}
			continue
		}

		r.extractTokenUsage(rest)

		if isAnthropicToolCallEvent(rest) {
			// Anthropic-format tool calls (content_block_start with type "tool_use").
			seenToolCalls = true
			anthropicBlockToTool = make(map[int]int)
			buffered = append(buffered, append([]byte(nil), line...))
			accumulateAnthropicToolCallInfo(rest, &toolCalls, anthropicBlockToTool)
		} else if isResponsesAPIToolCallEvent(rest) {
			// OpenAI Responses API tool calls (response.output_item.added with function_call).
			seenToolCalls = true
			responsesItemToTool = make(map[int]int)
			buffered = append(buffered, append([]byte(nil), line...))
			accumulateResponsesAPIToolCallInfo(rest, &toolCalls, responsesItemToTool)
		} else {
			_, _ = pw.Write(line)
		}

		if err != nil {
			break
		}
	}

	if len(toolCalls) == 0 {
		for _, line := range buffered {
			_, _ = pw.Write(line)
		}
		return
	}

	// Evaluate policies against tool calls.
	targetMessage := buildToolCallTargetMessage(toolCalls)
	logger.Infof("evaluating %d tool calls against %d policies", len(toolCalls), len(r.outputPolicies))
	violations := r.messagePolicyHelper.EvaluateMessage(ctx, r.outputPolicies, r.conversationHistory, targetMessage, types2.PolicyDirectionToolCalls)

	if len(violations) == 0 {
		for _, line := range buffered {
			_, _ = pw.Write(line)
		}
		return
	}

	// Log each violation.
	blockedContent, _ := json.Marshal(toolCalls)
	for _, v := range violations {
		logViolation(context.Background(), r.client, v, r.userID, string(types2.PolicyDirectionToolCalls), blockedContent, r.projectID, r.threadID)
	}

	// Violation — forward tool calls (to keep conversation history valid)
	// and inject a violation marker that nanobot can detect.
	var explanations []string
	for _, v := range violations {
		explanations = append(explanations, v.Explanation)
	}
	notification := fmt.Sprintf(
		"This tool call was blocked due to a policy violation. Please inform the user that you cannot complete their requested action. Explanation: %s",
		strings.Join(explanations, "\n"),
	)
	violationJSON, _ := json.Marshal(map[string]string{
		"obot_tool_call_policy_violation": notification,
	})
	violationLine := fmt.Sprintf("data: %s\n\n", violationJSON)

	// Flush buffered lines, injecting the violation marker before the stream terminator.
	// OpenAI Responses API ends with response.completed; Anthropic ends with content_block_stop.
	injected := false
	for _, line := range buffered {
		if !injected {
			rest, isData := bytes.CutPrefix(line, []byte("data: "))
			if isData {
				trimmed := bytes.TrimSpace(rest)
				eventType := gjson.GetBytes(trimmed, "type").String()
				isResponsesCompleted := eventType == "response.completed"
				isAnthropicStop := eventType == "content_block_stop"
				if isResponsesCompleted || isAnthropicStop {
					_, _ = pw.Write([]byte(violationLine))
					injected = true
				}
			}
		}
		_, _ = pw.Write(line)
	}
	if !injected {
		_, _ = pw.Write([]byte(violationLine))
	}
}

// streamAndEvaluateToolCallsJSON handles non-streaming (single JSON) responses.
func (r *responseModifier) streamAndEvaluateToolCallsJSON(ctx context.Context, pw *io.PipeWriter) {
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r.b)
	body := buf.Bytes()

	r.extractTokenUsage(body)

	// Anthropic format: content array with type "tool_use"
	anthropicContent := gjson.GetBytes(body, "content")
	// OpenAI Responses API format: output array with type "function_call"
	responsesOutput := gjson.GetBytes(body, "output")

	var toolCalls []messagepolicy.ToolCallInfo
	if responsesOutput.Exists() {
		responsesOutput.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "function_call" {
				toolCalls = append(toolCalls, messagepolicy.ToolCallInfo{
					Name:      item.Get("name").String(),
					Arguments: item.Get("arguments").String(),
				})
			}
			return true
		})
	} else if anthropicContent.Exists() {
		anthropicContent.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() == "tool_use" {
				input := block.Get("input").Raw
				if input == "" {
					input = "{}"
				}
				toolCalls = append(toolCalls, messagepolicy.ToolCallInfo{
					Name:      block.Get("name").String(),
					Arguments: input,
				})
			}
			return true
		})
	}

	if len(toolCalls) == 0 {
		_, _ = pw.Write(body)
		return
	}

	targetMessage := buildToolCallTargetMessage(toolCalls)
	violations := r.messagePolicyHelper.EvaluateMessage(ctx, r.outputPolicies, r.conversationHistory, targetMessage, types2.PolicyDirectionToolCalls)

	if len(violations) == 0 {
		_, _ = pw.Write(body)
		return
	}

	// Log each violation.
	blockedContent, _ := json.Marshal(toolCalls)
	for _, v := range violations {
		logViolation(context.Background(), r.client, v, r.userID, string(types2.PolicyDirectionToolCalls), blockedContent, r.projectID, r.threadID)
	}

	// Violation — keep tool calls but add violation marker to the JSON.
	var explanations []string
	for _, v := range violations {
		explanations = append(explanations, v.Explanation)
	}
	notification := fmt.Sprintf(
		"This tool call was blocked due to a policy violation. Please inform the user that you cannot complete their requested action. Explanation: %s",
		strings.Join(explanations, "\n"),
	)

	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		_, _ = pw.Write(body)
		return
	}
	bodyMap["obot_tool_call_policy_violation"] = notification
	modified, err := json.Marshal(bodyMap)
	if err != nil {
		_, _ = pw.Write(body)
		return
	}
	_, _ = pw.Write(append(modified, '\n'))
}

// isAnthropicToolCallEvent checks if an SSE data payload is an Anthropic tool-call-related event
// (content_block_start with type "tool_use", or content_block_delta with type "input_json_delta").
func isAnthropicToolCallEvent(data []byte) bool {
	eventType := gjson.GetBytes(data, "type").String()
	return (eventType == "content_block_start" && gjson.GetBytes(data, "content_block.type").String() == "tool_use") ||
		(eventType == "content_block_delta" && gjson.GetBytes(data, "delta.type").String() == "input_json_delta")
}

// accumulateAnthropicToolCallInfo extracts tool call name/arguments from Anthropic SSE events.
// content_block_start events with type "tool_use" create new entries;
// content_block_delta events with type "input_json_delta" append partial arguments.
func accumulateAnthropicToolCallInfo(data []byte, toolCalls *[]messagepolicy.ToolCallInfo, blockToTool map[int]int) {
	eventType := gjson.GetBytes(data, "type").String()
	switch {
	case eventType == "content_block_start" && gjson.GetBytes(data, "content_block.type").String() == "tool_use":
		blockIdx := int(gjson.GetBytes(data, "index").Int())
		toolIdx := len(*toolCalls)
		blockToTool[blockIdx] = toolIdx
		*toolCalls = append(*toolCalls, messagepolicy.ToolCallInfo{
			Name: gjson.GetBytes(data, "content_block.name").String(),
		})
	case eventType == "content_block_delta" && gjson.GetBytes(data, "delta.type").String() == "input_json_delta":
		blockIdx := int(gjson.GetBytes(data, "index").Int())
		if toolIdx, ok := blockToTool[blockIdx]; ok && toolIdx < len(*toolCalls) {
			(*toolCalls)[toolIdx].Arguments += gjson.GetBytes(data, "delta.partial_json").String()
		}
	}
}

// isResponsesAPIToolCallEvent checks if an SSE data payload is an OpenAI Responses API tool-call-related event
// (response.output_item.added with type "function_call", or response.function_call_arguments.delta).
func isResponsesAPIToolCallEvent(data []byte) bool {
	eventType := gjson.GetBytes(data, "type").String()
	return (eventType == "response.output_item.added" && gjson.GetBytes(data, "item.type").String() == "function_call") ||
		eventType == "response.function_call_arguments.delta"
}

// accumulateResponsesAPIToolCallInfo extracts tool call name/arguments from OpenAI Responses API SSE events.
// response.output_item.added events with item.type "function_call" create new entries;
// response.function_call_arguments.delta events append partial arguments.
func accumulateResponsesAPIToolCallInfo(data []byte, toolCalls *[]messagepolicy.ToolCallInfo, itemToTool map[int]int) {
	if itemToTool == nil {
		return
	}
	eventType := gjson.GetBytes(data, "type").String()
	switch {
	case eventType == "response.output_item.added" && gjson.GetBytes(data, "item.type").String() == "function_call":
		outputIdx := int(gjson.GetBytes(data, "output_index").Int())
		toolIdx := len(*toolCalls)
		itemToTool[outputIdx] = toolIdx
		*toolCalls = append(*toolCalls, messagepolicy.ToolCallInfo{
			Name: gjson.GetBytes(data, "item.name").String(),
		})
	case eventType == "response.function_call_arguments.delta":
		outputIdx := int(gjson.GetBytes(data, "output_index").Int())
		if toolIdx, ok := itemToTool[outputIdx]; ok && toolIdx < len(*toolCalls) {
			(*toolCalls)[toolIdx].Arguments += gjson.GetBytes(data, "delta").String()
		}
	}
}

// buildToolCallTargetMessage formats tool calls into the target message string for the policy judge.
// logViolation persists a policy violation record. Failures are logged but non-fatal.
func logViolation(ctx context.Context, c *client.Client, v messagepolicy.MessagePolicyViolation, userID, direction string, blockedContent json.RawMessage, projectID, threadID string) {
	if c == nil {
		return
	}
	if err := c.LogMessagePolicyViolation(ctx, &types.MessagePolicyViolation{
		CreatedAt:            time.Now(),
		UserID:               userID,
		PolicyID:             v.PolicyID,
		PolicyName:           v.PolicyName,
		PolicyDefinition:     v.PolicyDefinition,
		Direction:            direction,
		ViolationExplanation: v.Explanation,
		BlockedContent:       blockedContent,
		ProjectID:            projectID,
		ThreadID:             threadID,
	}); err != nil {
		logger.Warnf("failed to log policy violation for policy %s: %v", v.PolicyID, err)
	}
}

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
		req.Header.Del(internalRequestTypeHeader)
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

		itemType, _ := msg["type"].(string)

		// OpenAI Responses API: top-level function_call items.
		if itemType == "function_call" {
			name, _ := msg["name"].(string)
			arguments, _ := msg["arguments"].(string)
			history = append(history, messagepolicy.ConversationMessage{
				Role: "assistant",
				ToolCalls: []messagepolicy.ToolCallInfo{
					{Name: name, Arguments: arguments},
				},
			})
			continue
		}

		// OpenAI Responses API: top-level function_call_output items.
		if itemType == "function_call_output" {
			callID, _ := msg["call_id"].(string)
			output, _ := msg["output"].(string)
			history = append(history, messagepolicy.ConversationMessage{
				Role:       "tool",
				Content:    output,
				ToolCallID: callID,
			})
			continue
		}

		role, _ := msg["role"].(string)
		content := extractContentString(msg["content"])

		cm := messagepolicy.ConversationMessage{
			Role:    role,
			Content: content,
		}

		history = append(history, cm)

		if role == "user" && content != "" {
			lastUserMsg = content
			lastUserIdx = i
		}
	}

	return history, lastUserMsg, lastUserIdx
}

// extractRawMessages extracts the messages array from a request body map.
// For Anthropic, this is the "messages" field.
// For OpenAI Responses API, this is the "input" field (when it's an array).
func extractRawMessages(bodyMap map[string]any) []any {
	if messages, ok := bodyMap["messages"].([]any); ok {
		return messages
	}
	if input, ok := bodyMap["input"].([]any); ok {
		return input
	}
	return nil
}

// extractContentString extracts a text string from a message content field,
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
			partType, _ := partMap["type"].(string)
			switch partType {
			case "text", "input_text", "output_text":
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
		model, err := getModelFromReference(req.Context(), req.Storage, l.modelProvider.Namespace, targetModel)
		if err != nil {
			return fmt.Errorf("failed to get model: %w", err)
		}
		if model.Spec.Manifest.ModelProvider != l.modelProvider.Name {
			return types2.NewErrBadRequest("requested model does not match configured provider %q", targetModel)
		}

		hasAccess, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name)
		if err != nil {
			return fmt.Errorf("failed to check user access to model %q: %w", model.Name, err)
		}
		if !hasAccess {
			return types2.NewErrForbidden("user does not have permission to use model %q", targetModel)
		}

		targetModel = model.Spec.Manifest.TargetModel

		// Replace the model resource name with the actual provider model name
		body, err = rewriteModelInBody(body, targetModel)
		if err != nil {
			return fmt.Errorf("failed to rewrite model in request body: %w", err)
		}
		req.Request.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}

	// Evaluate message policies if the helper is available and we have a user.
	var (
		messagePolicyHelper    = l.messagePolicyHelper
		outputPolicies         []messagepolicy.ApplicablePolicy
		conversationHistory    []messagepolicy.ConversationMessage
		inputPolicyReplacement string
	)
	if shouldSkipMessagePolicyEnforcement(req.Request) {
		messagePolicyHelper = nil
	}
	if messagePolicyHelper != nil && req.User.GetUID() != "" {
		var bodyMap map[string]any
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			outputPolicies, conversationHistory, inputPolicyReplacement, err = applyMessagePolicies(
				req.Context(), messagePolicyHelper, req.User, req.GatewayClient, bodyMap, "", "",
			)
			if err != nil {
				return err
			}
			if inputPolicyReplacement != "" {
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
			userID:                 req.User.GetUID(),
			model:                  targetModel,
			client:                 req.GatewayClient,
			inputPolicyReplacement: inputPolicyReplacement,
			messagePolicyHelper:    messagePolicyHelper,
			outputPolicies:         outputPolicies,
			conversationHistory:    conversationHistory,
		}).modifyResponse,
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

// applyMessagePolicies evaluates input and output message policies against body, modifying
// it in-place if an input policy is violated (replacing the last user message with an LLM
// refusal). Returns the output policies to enforce on the response, the conversation history
// for output policy evaluation, the user-facing replacement text to surface via response
// header, and any error.
func applyMessagePolicies(
	ctx context.Context,
	helper *messagepolicy.Helper,
	userInfo user.Info,
	gatewayClient *client.Client,
	body map[string]any,
	projectID, threadID string,
) ([]messagepolicy.ApplicablePolicy, []messagepolicy.ConversationMessage, string, error) {
	inputPolicies, err := helper.GetApplicablePolicies(userInfo, types2.PolicyDirectionUserMessage)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get applicable input policies: %w", err)
	}

	var inputPolicyReplacement string
	if len(inputPolicies) > 0 {
		rawMessages := extractRawMessages(body)
		if len(rawMessages) > 0 {
			history, lastUserMsg, lastUserIdx := parseMessagesFromBody(rawMessages)
			// Only evaluate when the user message is last in the conversation. If there are
			// messages after it (assistant responses, tool results, etc.), this is a
			// tool-calling continuation and the user text has already been evaluated.
			if lastUserIdx == len(rawMessages)-1 {
				violations := helper.EvaluateMessage(ctx, inputPolicies, history, lastUserMsg, types2.PolicyDirectionUserMessage)
				if len(violations) > 0 {
					blockedContent, _ := json.Marshal(map[string]string{"message": lastUserMsg})
					var explanations []string
					for _, v := range violations {
						logViolation(ctx, gatewayClient, v, userInfo.GetUID(), string(types2.PolicyDirectionUserMessage), blockedContent, projectID, threadID)
						explanations = append(explanations, v.Explanation)
					}
					if msgMap, ok := rawMessages[lastUserIdx].(map[string]any); ok {
						msgMap["content"] = `Please respond to the user with exactly: "Sorry, I can't help with that."`
						inputPolicyReplacement = fmt.Sprintf("[policy-violation] %s", strings.Join(explanations, "\n"))
					}
				}
			}
		}
	}

	outputPolicies, err := helper.GetApplicablePolicies(userInfo, types2.PolicyDirectionToolCalls)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get applicable output policies: %w", err)
	}

	var conversationHistory []messagepolicy.ConversationMessage
	if len(outputPolicies) > 0 {
		rawMessages := extractRawMessages(body)
		conversationHistory, _, _ = parseMessagesFromBody(rawMessages)
	}

	return outputPolicies, conversationHistory, inputPolicyReplacement, nil
}

func shouldSkipMessagePolicyEnforcement(req *http.Request) bool {
	if req == nil {
		return false
	}

	return req.Header.Get(internalRequestTypeHeader) == threadTitleRequestType
}
