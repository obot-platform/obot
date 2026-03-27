package messagepolicy

import (
	"context"
	"fmt"
	"strings"
	"sync"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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

// PolicyViolation is the result when a message violates a policy.
type PolicyViolation struct {
	PolicyName  string
	Explanation string
}

// resolvedModel holds pre-resolved model provider information to avoid redundant lookups.
type resolvedModel struct {
	targetModel string
	providerURL string
	credHeaders map[string]string
}

// EvaluateMessage runs all applicable policies against a message in parallel.
// Returns a slice of violations (empty if all policies pass). Never returns an error;
// LLM failures are treated as violations (fail closed).
func (h *Helper) EvaluateMessage(ctx context.Context, policies []types.MessagePolicyManifest, conversationHistory []ConversationMessage, targetMessage string, direction types.PolicyDirection) []PolicyViolation {
	if len(policies) == 0 {
		return nil
	}

	log.Debugf("Evaluating %d message policies for direction=%s", len(policies), direction)

	// Resolve model once for all policy evaluations.
	resolved, err := h.resolveModel(ctx)
	if err != nil {
		log.Errorf("Failed to resolve llm-mini model for policy evaluation, failing closed: %v", err)
		// If we can't resolve the model, fail closed: report a validation error for each policy.
		var violations []PolicyViolation
		for _, p := range policies {
			violations = append(violations, PolicyViolation{
				PolicyName:  p.DisplayName,
				Explanation: fmt.Sprintf("An error occurred while validating the message against the policy %q. The message has been blocked as a precaution.", p.DisplayName),
			})
		}
		return violations
	}

	log.Debugf("Resolved llm-mini to model=%s provider=%s", resolved.targetModel, resolved.providerURL)

	conversationContext := BuildConversationContext(conversationHistory)

	var (
		mu         sync.Mutex
		violations []PolicyViolation
		wg         sync.WaitGroup
	)

	for _, policy := range policies {
		wg.Add(1)
		go func(p types.MessagePolicyManifest) {
			defer wg.Done()

			compliant := h.checkCompliance(ctx, resolved, p, conversationContext, targetMessage)
			if !compliant {
				log.Warnf("Policy violation detected: policy=%q", p.DisplayName)
				explanation := h.generateExplanation(ctx, resolved, p, targetMessage, direction)

				mu.Lock()
				violations = append(violations, PolicyViolation{
					PolicyName:  p.DisplayName,
					Explanation: explanation,
				})
				mu.Unlock()
			} else {
				log.Debugf("Message compliant with policy=%q", p.DisplayName)
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
	log.Debugf("Resolving model alias %s", types.DefaultModelAliasTypeLLMMini)

	m, err := alias.GetFromScope(ctx, h.client, "Model", system.DefaultNamespace, string(types.DefaultModelAliasTypeLLMMini))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve llm-mini alias: %w", err)
	}

	var model *v1.Model
	switch resolved := m.(type) {
	case *v1.DefaultModelAlias:
		if resolved.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("default model alias %q is not configured", types.DefaultModelAliasTypeLLMMini)
		}
		var mdl v1.Model
		if err := alias.Get(ctx, h.client, &mdl, system.DefaultNamespace, resolved.Spec.Manifest.Model); err != nil {
			return nil, fmt.Errorf("failed to get model from alias: %w", err)
		}
		model = &mdl
	case *v1.Model:
		model = resolved
	default:
		return nil, fmt.Errorf("unexpected type %T when resolving llm-mini", m)
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

	return &resolvedModel{
		targetModel: model.Spec.Manifest.TargetModel,
		providerURL: providerURL.String(),
		credHeaders: credHeaders,
	}, nil
}

// callLLM makes a chat completion call to the resolved model provider.
func (h *Helper) callLLM(ctx context.Context, resolved *resolvedModel, messages []openai.ChatCompletionMessage) (string, error) {
	cfg := openai.DefaultConfig("")
	cfg.BaseURL = resolved.providerURL + "/v1"

	client := openai.NewClientWithConfig(cfg)

	log.Debugf("Making LLM call to model=%s", resolved.targetModel)

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    resolved.targetModel,
		Messages: messages,
	}, resolved.credHeaders)
	if err != nil {
		log.Errorf("LLM call to model=%s failed: %v", resolved.targetModel, err)
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Errorf("LLM call to model=%s returned no choices", resolved.targetModel)
		return "", fmt.Errorf("LLM returned no choices")
	}

	log.Debugf("LLM call to model=%s succeeded", resolved.targetModel)
	return resp.Choices[0].Message.Content, nil
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

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: "You are a policy compliance checker. You must determine whether the following message violates the given policy. Respond with exactly \"yes\" if the message is compliant, or \"no\" if it violates the policy. Do not explain your reasoning.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
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

// generateExplanation performs Stage 2: generates a human-readable explanation for a violation.
// On error, returns a generic explanation.
func (h *Helper) generateExplanation(ctx context.Context, resolved *resolvedModel, policy types.MessagePolicyManifest, targetMessage string, direction types.PolicyDirection) string {
	log.Debugf("Generating violation explanation for policy=%q direction=%s", policy.DisplayName, direction)
	var audienceInstruction string
	switch direction {
	case types.PolicyDirectionLLMResponse:
		audienceInstruction = "Write the explanation directly for the end user."
	default:
		audienceInstruction = "Write the explanation for an AI assistant to relay to the user."
	}

	userContent := fmt.Sprintf("Policy: %s\n\nMessage that was blocked:\n%s", policy.Definition, targetMessage)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: fmt.Sprintf("A message was blocked for violating a policy. Write a brief explanation of why the message was blocked. %s", audienceInstruction),
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userContent,
		},
	}

	result, err := h.callLLM(ctx, resolved, messages)
	if err != nil {
		log.Warnf("Explanation generation LLM call failed for policy=%q, using generic explanation: %v", policy.DisplayName, err)
		return fmt.Sprintf("This message was blocked for violating the policy: %s", policy.DisplayName)
	}

	log.Debugf("Generated explanation for policy=%q", policy.DisplayName)
	return result
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
