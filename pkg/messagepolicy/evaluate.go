package messagepolicy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ConversationMessage represents a message in conversation history for policy evaluation.
type ConversationMessage struct {
	Role       string // "user", "assistant", "tool", "system"
	Content    string
	ToolCalls  []ToolCallInfo
	ToolCallID string
}

// ToolCallInfo contains the name and arguments of a tool call.
type ToolCallInfo struct {
	Name      string
	Arguments string
}

// MessagePolicyViolation is the result when a message violates a policy.
// nolint:revive
type MessagePolicyViolation struct {
	PolicyID         string
	PolicyName       string
	PolicyDefinition string
	Explanation      string
}

// resolvedModel holds pre-resolved model provider information to avoid redundant lookups.
type resolvedModel struct {
	targetModel string
	providerURL string
	credHeaders map[string]string
	dialect     string
}

// EvaluateMessage runs all applicable policies against a message in parallel.
// Returns a slice of violations (empty if all policies pass). Never returns an error;
// LLM failures are treated as violations (fail closed).
func (h *Helper) EvaluateMessage(ctx context.Context, policies []ApplicablePolicy, conversationHistory []ConversationMessage, targetMessage string, direction types.PolicyDirection) []MessagePolicyViolation {
	if len(policies) == 0 {
		return nil
	}

	log.Debugf("Evaluating %d message policies for direction=%s", len(policies), direction)

	// Resolve model once for all policy evaluations.
	resolved, err := h.resolveModel(ctx)
	if err != nil {
		log.Errorf("Failed to resolve llm-mini model for policy evaluation, failing closed: %v", err)
		// If we can't resolve the model, fail closed: report a validation error for each policy.
		var violations []MessagePolicyViolation
		for _, p := range policies {
			violations = append(violations, MessagePolicyViolation{
				PolicyID:         p.ID,
				PolicyName:       p.Manifest.DisplayName,
				PolicyDefinition: p.Manifest.Definition,
				Explanation:      fmt.Sprintf("An error occurred while validating the message against the policy %q. The message has been blocked as a precaution.", p.Manifest.DisplayName),
			})
		}
		return violations
	}

	log.Debugf("Resolved llm-mini to model=%s provider=%s", resolved.targetModel, resolved.providerURL)

	conversationContext := BuildConversationContext(conversationHistory)

	var (
		mu         sync.Mutex
		violations []MessagePolicyViolation
		wg         sync.WaitGroup
	)

	for _, policy := range policies {
		wg.Add(1)
		go func(p ApplicablePolicy) {
			defer wg.Done()

			compliant := h.checkCompliance(ctx, resolved, p.Manifest, conversationContext, targetMessage)
			if !compliant {
				log.Infof("Policy violation detected by stage 1 for policy=%q, running stage 2 review", p.Manifest.DisplayName)

				// Stage 2: Use the full Chat model to review the denial and potentially override it.
				// If it upholds the denial, it also provides the user-facing explanation.
				review := h.reviewCompliance(ctx, p.Manifest, conversationContext, targetMessage)
				if review.Compliant {
					log.Infof("Stage 2 review overrode stage 1 denial for policy=%q", p.Manifest.DisplayName)
					return
				}

				log.Infof("Stage 2 review confirmed violation for policy=%q", p.Manifest.DisplayName)

				mu.Lock()
				violations = append(violations, MessagePolicyViolation{
					PolicyID:         p.ID,
					PolicyName:       p.Manifest.DisplayName,
					PolicyDefinition: p.Manifest.Definition,
					Explanation:      review.Explanation,
				})
				mu.Unlock()
			} else {
				log.Debugf("Message compliant with policy=%q", p.Manifest.DisplayName)
			}
		}(policy)
	}

	wg.Wait()

	if len(violations) > 0 {
		log.Infof("Policy evaluation complete: %d violation(s) found", len(violations))
	} else {
		log.Debugf("Policy evaluation complete: no violations found")
	}

	return violations
}

// resolveModel resolves the llm-mini alias to get the target model name, provider URL, and credential headers.
func (h *Helper) resolveModel(ctx context.Context) (*resolvedModel, error) {
	return h.resolveModelByAlias(ctx, types.DefaultModelAliasTypeLLMMini)
}

// resolveModelByAlias resolves the given model alias to get the target model name, provider URL, and credential headers.
func (h *Helper) resolveModelByAlias(ctx context.Context, aliasType types.DefaultModelAliasType) (*resolvedModel, error) {
	log.Debugf("Resolving model alias %s", aliasType)

	m, err := alias.GetFromScope(ctx, h.client, "Model", system.DefaultNamespace, string(aliasType))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s alias: %w", aliasType, err)
	}

	var model *v1.Model
	switch resolved := m.(type) {
	case *v1.DefaultModelAlias:
		if resolved.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", aliasType)
		}
		var mdl v1.Model
		if err := alias.Get(ctx, h.client, &mdl, system.DefaultNamespace, resolved.Spec.Manifest.Model); err != nil {
			return nil, fmt.Errorf("failed to get model from alias: %w", err)
		}
		model = &mdl
	case *v1.Model:
		model = resolved
	default:
		return nil, fmt.Errorf("unexpected type %T when resolving %s", m, aliasType)
	}

	if !model.Spec.Manifest.Active {
		return nil, fmt.Errorf("model %q is not active", model.Spec.Manifest.Name)
	}

	providerURL, err := h.dispatcher.URLForModelProvider(ctx, h.gptClient, system.DefaultNamespace, model.Spec.Manifest.ModelProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get model provider URL: %w", err)
	}

	var toolRef v1.ToolReference
	if err := h.client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: model.Spec.Manifest.ModelProvider}, &toolRef); err != nil {
		return nil, fmt.Errorf("failed to get model provider tool reference: %w", err)
	}

	credEnv, err := dispatcher.CredentialEnvForModelProvider(ctx, h.gptClient, toolRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get model provider credentials: %w", err)
	}

	credHeaders := make(map[string]string, len(credEnv))
	for k, v := range credEnv {
		credHeaders[fmt.Sprintf("X-Obot-%s", k)] = v
	}

	// only add /v1 if the URL has no path.
	if providerURL.Path == "" || providerURL.Path == "/" {
		providerURL.Path = "/v1"
	}

	return &resolvedModel{
		targetModel: model.Spec.Manifest.TargetModel,
		providerURL: providerURL.String(),
		credHeaders: credHeaders,
		dialect:     model.Spec.Manifest.Dialect,
	}, nil
}

// chatMessage is a minimal OpenAI-format chat message for policy evaluation requests.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest is the request body for OpenAI-format chat completions.
// Stream is always set to true to match normal Obot proxy usage patterns.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// bifrostLLMRequest is a minimal Bifrost-compatible /v1/responses request body.
// The daemon overrides the Provider field internally, so it does not need to be set.
type bifrostLLMRequest struct {
	Model  string           `json:"model"`
	Input  []bifrostMessage `json:"input"`
	Params *bifrostParams   `json:"params,omitempty"`
}

type bifrostMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type bifrostParams struct {
	Instructions string `json:"instructions,omitempty"`
}

// callLLM makes a streaming LLM call to the resolved model provider, using the
// appropriate API format for the provider's dialect.
func (h *Helper) callLLM(ctx context.Context, resolved *resolvedModel, messages []chatMessage) (string, error) {
	if resolved.dialect == string(nanobottypes.DialectBifrostRequest) {
		return h.callLLMBifrost(ctx, resolved, messages)
	}
	return h.callLLMChatCompletions(ctx, resolved, messages)
}

// callLLMChatCompletions calls the provider using the OpenAI chat completions format.
func (h *Helper) callLLMChatCompletions(ctx context.Context, resolved *resolvedModel, messages []chatMessage) (string, error) {
	reqBody := chatCompletionRequest{
		Model:    resolved.targetModel,
		Messages: messages,
		Stream:   true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := resolved.providerURL + "/chat/completions"
	log.Debugf("Making LLM call to model=%s url=%s", resolved.targetModel, reqURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range resolved.credHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Errorf("LLM call to model=%s failed: %v", resolved.targetModel, err)
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Errorf("LLM call to model=%s returned status %d: %s", resolved.targetModel, resp.StatusCode, string(respBody))
		return "", fmt.Errorf("LLM call returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return readStreamingResponse(resp.Body, func(data string) gjson.Result {
		return gjson.Get(data, "choices.0.delta.content")
	})
}

// callLLMBifrost calls a Bifrost-dialect provider using the /v1/responses endpoint.
// The Bifrost daemon overrides the Provider field from its own configuration, so
// the client does not need to specify it.
func (h *Helper) callLLMBifrost(ctx context.Context, resolved *resolvedModel, messages []chatMessage) (string, error) {
	var (
		systemPrompt string
		input        []bifrostMessage
	)
	for _, m := range messages {
		if m.Role == "system" {
			systemPrompt = m.Content
		} else {
			input = append(input, bifrostMessage(m))
		}
	}

	reqBody := bifrostLLMRequest{
		Model: resolved.targetModel,
		Input: input,
	}
	if systemPrompt != "" {
		reqBody.Params = &bifrostParams{Instructions: systemPrompt}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal bifrost request: %w", err)
	}

	reqURL := strings.TrimSuffix(resolved.providerURL, "/")
	if !strings.HasSuffix(reqURL, "/v1") {
		reqURL += "/v1"
	}
	reqURL = reqURL + "/responses"
	log.Debugf("Making LLM call to model=%s url=%s (bifrost)", resolved.targetModel, reqURL)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range resolved.credHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		log.Errorf("LLM call to model=%s failed: %v", resolved.targetModel, err)
		return "", fmt.Errorf("LLM call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Errorf("LLM call to model=%s returned status %d: %s", resolved.targetModel, resp.StatusCode, string(respBody))
		return "", fmt.Errorf("LLM call returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return readStreamingResponse(resp.Body, func(data string) gjson.Result {
		if gjson.Get(data, "type").String() == "response.output_text.delta" {
			return gjson.Get(data, "delta")
		}
		return gjson.Result{}
	})
}

// readStreamingResponse reads a generic SSE stream, calling extractDelta for each data line.
// If the returned Result exists, its string value is appended to the output.
func readStreamingResponse(r io.Reader, extractDelta func(data string) gjson.Result) (string, error) {
	var content strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1024*1024) // 1MB max to handle large SSE data lines

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		if delta := extractDelta(data); delta.Exists() {
			content.WriteString(delta.String())
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read SSE stream: %w", err)
	}

	return content.String(), nil
}

// checkCompliance performs Stage 1: a yes/no compliance check. Returns true if compliant.
// Fails closed: any error or ambiguous response is treated as a violation.
func (h *Helper) checkCompliance(ctx context.Context, resolved *resolvedModel, policy types.MessagePolicyManifest, conversationContext, targetMessage string) bool {
	log.Debugf("Checking compliance for policy=%q", policy.DisplayName)
	var userContent strings.Builder
	fmt.Fprintf(&userContent, "Policy: %s\n\n---\n\n", policy.Definition)

	if conversationContext != "" {
		fmt.Fprintf(&userContent, "Conversation context (tool outputs redacted):\n%s\n\n---\n\n", conversationContext)
	}

	fmt.Fprintf(&userContent, "Message to evaluate:\n%s", targetMessage)

	messages := []chatMessage{
		{
			Role: "system",
			Content: "You are a policy compliance checker. You will be given a policy, a conversation history for context, and a final message to evaluate. " +
				"You must determine whether the FINAL MESSAGE — and ONLY the final message — violates the given policy. " +
				"The conversation history is provided solely for context. Even if earlier messages in the conversation violated the policy, " +
				"that does NOT mean the final message is also in violation. Evaluate the final message on its own merits. " +
				"Respond with exactly \"yes\" if the final message is compliant, or \"no\" if it violates the policy. Do not explain your reasoning.",
		},
		{
			Role:    "user",
			Content: userContent.String(),
		},
	}

	result, err := h.callLLM(ctx, resolved, messages)
	if err != nil {
		log.Warnf("Compliance check LLM call failed for policy=%q, failing closed: %v", policy.DisplayName, err)
		return false // fail closed
	}

	answer := strings.TrimSpace(strings.ToLower(result))
	if answer != "yes" && answer != "no" {
		log.Warnf("Unexpected compliance check response for policy=%q: %q, treating as violation", policy.DisplayName, result)
	}

	return answer == "yes"
}

// reviewResult holds the outcome of a Stage 2 review.
type reviewResult struct {
	Compliant   bool
	Explanation string
}

// reviewCompliance performs Stage 2: uses the full Chat model (llm alias) to review a stage 1 denial.
// Returns the review result with a compliance decision and, if denied, a user-facing explanation.
// Fails closed: any error returns a non-compliant result with a generic explanation.
func (h *Helper) reviewCompliance(ctx context.Context, policy types.MessagePolicyManifest, conversationContext, targetMessage string) reviewResult {
	log.Debugf("Reviewing stage 1 denial for policy=%q using Chat model", policy.DisplayName)

	genericDenial := reviewResult{
		Compliant:   false,
		Explanation: fmt.Sprintf("Your message was blocked by the \"%s\" message policy.", policy.DisplayName),
	}

	resolved, err := h.resolveModelByAlias(ctx, types.DefaultModelAliasTypeLLM)
	if err != nil {
		log.Warnf("Failed to resolve llm model for stage 2 review of policy=%q, upholding denial: %v", policy.DisplayName, err)
		return genericDenial
	}

	var userContent strings.Builder
	fmt.Fprintf(&userContent, "Policy: %s\n\n---\n\n", policy.Definition)

	if conversationContext != "" {
		fmt.Fprintf(&userContent, "Conversation context (tool outputs redacted):\n%s\n\n---\n\n", conversationContext)
	}

	fmt.Fprintf(&userContent, "Message to evaluate:\n%s", targetMessage)

	messages := []chatMessage{
		{
			Role: "system",
			Content: "You are a senior policy compliance reviewer. A fast screening model has flagged a message as potentially violating a policy. " +
				"Your job is to carefully review whether the message ACTUALLY violates the policy. " +
				"You will be given the policy, a conversation history for context, and the flagged message. " +
				"Evaluate ONLY the flagged message against the policy. The conversation history is provided solely for context. " +
				"Think carefully about edge cases, nuance, and whether the message truly violates the spirit of the policy.\n\n" +
				"You MUST respond with EXACTLY one of these two formats:\n" +
				"- If the message is compliant (the screening was a false positive), respond with exactly: COMPLIANT\n" +
				"- If the message genuinely violates the policy, respond with: VIOLATION: followed by a short explanation " +
				"that will be displayed to the user in a \"Policy Violation\" notice. " +
				"Address the user directly (use \"your message\", not \"the message\"). " +
				"Do not reference \"the policy\" or \"this policy\" — the user does not know what policy blocked them. " +
				"Instead, describe the rule in plain language (e.g. \"profane language is not allowed\" instead of \"this policy forbids profane language\"). " +
				"Do not use phrases like \"I'd explain\" or wrap your output in quotes.\n\n" +
				"Examples:\n" +
				"COMPLIANT\n" +
				"VIOLATION: Your message contains personal contact information, which is not allowed.",
		},
		{
			Role:    "user",
			Content: userContent.String(),
		},
	}

	result, err := h.callLLM(ctx, resolved, messages)
	if err != nil {
		log.Warnf("Stage 2 review LLM call failed for policy=%q, upholding denial: %v", policy.DisplayName, err)
		return genericDenial
	}

	return parseReviewResponse(result, genericDenial)
}

// parseReviewResponse parses the Stage 2 model output into a reviewResult.
// Expects either "COMPLIANT" or "VIOLATION: <explanation>".
// Falls back to the provided genericDenial if the format is unrecognized.
func parseReviewResponse(response string, genericDenial reviewResult) reviewResult {
	response = strings.TrimSpace(response)

	if strings.EqualFold(response, "COMPLIANT") {
		return reviewResult{Compliant: true}
	}

	upper := strings.ToUpper(response)
	if strings.HasPrefix(upper, "VIOLATION:") {
		explanation := strings.TrimSpace(response[len("VIOLATION:"):])
		if explanation == "" {
			return genericDenial
		}
		return reviewResult{Compliant: false, Explanation: explanation}
	}

	log.Warnf("Unexpected stage 2 review response format, upholding denial: %q", response)
	return genericDenial
}

// BuildConversationContext formats conversation history for the policy judge.
// System messages are excluded. Tool outputs are replaced with "[tool output redacted]".
func BuildConversationContext(messages []ConversationMessage) string {
	var parts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			continue
		case "user":
			parts = append(parts, fmt.Sprintf("User: %s", msg.Content))
		case "assistant":
			if msg.Content != "" {
				parts = append(parts, fmt.Sprintf("Assistant: %s", msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				parts = append(parts, fmt.Sprintf("Assistant: [called tool %q with args: %s]", tc.Name, tc.Arguments))
			}
		case "tool":
			parts = append(parts, "Tool: [tool output redacted]")
		}
	}

	return strings.Join(parts, "\n")
}
