package messagepolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// completionEngine keeps the judge independent of gateway lifecycle and policy
// plugins; both call into the judge, so importing the engine would create a cycle.
type completionEngine interface {
	ChatCompletion(context.Context, string, []schemas.ChatMessage) (*schemas.BifrostChatResponse, error)
}

// streamingCompletionEngine is implemented by gateways that stream chat completions.
type streamingCompletionEngine interface {
	ChatCompletionStream(context.Context, string, []schemas.ChatMessage, *schemas.ChatParameters, func(*schemas.ChatStreamResponseChoiceDelta)) error
}

// structuredCompletionEngine is implemented by gateways that accept request parameters,
// such as response_format.
type structuredCompletionEngine interface {
	ChatCompletionWithParams(context.Context, string, []schemas.ChatMessage, *schemas.ChatParameters) (*schemas.BifrostChatResponse, error)
}

// Helper evaluates natural-language policies with an LLM judge: the LLM judge guardrail,
// which message policies became.
type Helper struct {
	client     kclient.Client
	llmGateway completionEngine
}

// ApplicablePolicy pairs a policy's ID with its manifest.
type ApplicablePolicy struct {
	ID       string
	Manifest types.MessagePolicyManifest
}

// NewHelper returns a judge that calls the default model aliases' models through the LLM
// gateway.
func NewHelper(client kclient.Client, llmGateway completionEngine) *Helper {
	return &Helper{client: client, llmGateway: llmGateway}
}

// Complete makes one chat completion through the LLM gateway and returns the model's text
// and the gateway model that answered. It is the primitive the provenance Supervisor builds
// on.
//
// model is a default model alias ("llm-mini", "llm"; "" means "llm-mini"), the name of a
// Model in storage, or a gateway model id ("openai/gpt-5-mini") sent through as is. When
// jsonSchema is set the output is constrained to it with response_format, which the gateway
// translates for every provider; a provider that refuses the request is asked once more
// without it, so callers must still parse the text defensively.
func (h *Helper) Complete(ctx context.Context, model, system, user string, jsonSchema map[string]any) (string, string, error) {
	resolved, err := h.resolveNamedModel(ctx, model)
	if err != nil {
		return "", "", err
	}
	text, err := h.complete(ctx, resolved, system, user, jsonSchema)
	return text, resolved.targetModel, err
}

func (h *Helper) complete(ctx context.Context, resolved *resolvedModel, system, user string, jsonSchema map[string]any) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	structured, ok := h.llmGateway.(structuredCompletionEngine)
	if jsonSchema == nil || !ok {
		return h.callLLM(ctx, resolved, messages)
	}
	var format any = map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "response",
			"schema": jsonSchema,
		},
	}
	resp, err := structured.ChatCompletionWithParams(ctx, resolved.targetModel, toBifrostMessages(messages), &schemas.ChatParameters{ResponseFormat: &format})
	if err != nil {
		if ctx.Err() != nil {
			return "", err
		}
		// The provider may have refused structured output; the prompt asks for JSON anyway.
		slog.Debug("Structured output failed, retrying without response_format", "model", resolved.targetModel, "error", err)
		return h.callLLM(ctx, resolved, messages)
	}
	return responseText(resp)
}

// resolveNamedModel resolves a model name as Complete documents it.
func (h *Helper) resolveNamedModel(ctx context.Context, model string) (*resolvedModel, error) {
	switch model {
	case "", string(types.DefaultModelAliasTypeLLMMini):
		return h.resolveModelByAlias(ctx, types.DefaultModelAliasTypeLLMMini)
	case string(types.DefaultModelAliasTypeLLM):
		return h.resolveModelByAlias(ctx, types.DefaultModelAliasTypeLLM)
	}
	if h.llmGateway == nil {
		return nil, errors.New("the LLM gateway is not available")
	}
	target := model
	if h.client != nil {
		var mdl v1.Model
		err := alias.Get(ctx, h.client, &mdl, system.DefaultNamespace, model)
		switch {
		case err == nil:
			if !mdl.Spec.Manifest.Active {
				return nil, fmt.Errorf("model %q is not active", mdl.Spec.Manifest.Name)
			}
			if mdl.Spec.Manifest.ModelProvider != system.LLMGatewayModelProvider {
				return nil, fmt.Errorf("model %q is not served by the LLM gateway", mdl.Name)
			}
			target = mdl.Spec.Manifest.TargetModel
		case !apierrors.IsNotFound(err):
			return nil, fmt.Errorf("failed to look up model %q: %w", model, err)
		}
	}
	return &resolvedModel{targetModel: target}, nil
}

// ChatTurn sends one turn of a tool-using conversation through the LLM gateway and returns
// the assistant's message, which may call tools, and the gateway model that answered. model
// is resolved as Complete documents.
func (h *Helper) ChatTurn(ctx context.Context, model string, messages []schemas.ChatMessage, tools []schemas.ChatTool) (*schemas.ChatMessage, string, error) {
	resolved, err := h.resolveNamedModel(ctx, model)
	if err != nil {
		return nil, "", err
	}
	structured, ok := h.llmGateway.(structuredCompletionEngine)
	if !ok {
		return nil, "", errors.New("the LLM gateway does not support tool calls")
	}
	var params *schemas.ChatParameters
	if len(tools) > 0 {
		params = &schemas.ChatParameters{Tools: tools}
	}
	resp, err := structured.ChatCompletionWithParams(ctx, resolved.targetModel, messages, params)
	if err != nil {
		return nil, resolved.targetModel, fmt.Errorf("LLM call failed: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return nil, resolved.targetModel, errors.New("LLM call returned no message")
	}
	return resp.Choices[0].Message, resolved.targetModel, nil
}

// ChatTurnStream is ChatTurn streamed: onText receives the assistant's text as it is
// written. The returned message, including any tool calls, is assembled from the stream.
func (h *Helper) ChatTurnStream(ctx context.Context, model string, messages []schemas.ChatMessage, tools []schemas.ChatTool, onText func(string)) (*schemas.ChatMessage, string, error) {
	resolved, err := h.resolveNamedModel(ctx, model)
	if err != nil {
		return nil, "", err
	}
	streaming, ok := h.llmGateway.(streamingCompletionEngine)
	if !ok {
		return nil, resolved.targetModel, errors.New("the LLM gateway does not support streaming")
	}
	var params *schemas.ChatParameters
	if len(tools) > 0 {
		params = &schemas.ChatParameters{Tools: tools}
	}
	var acc streamAccumulator
	if err := streaming.ChatCompletionStream(ctx, resolved.targetModel, messages, params, func(d *schemas.ChatStreamResponseChoiceDelta) {
		if text := acc.add(d); text != "" && onText != nil {
			onText(text)
		}
	}); err != nil {
		return nil, resolved.targetModel, fmt.Errorf("LLM call failed: %w", err)
	}
	return acc.message(), resolved.targetModel, nil
}

// streamAccumulator assembles a streamed assistant message.
type streamAccumulator struct {
	text  strings.Builder
	calls []schemas.ChatAssistantMessageToolCall
}

// add folds a delta into the message and returns its new text.
func (a *streamAccumulator) add(d *schemas.ChatStreamResponseChoiceDelta) string {
	var text string
	if d.Content != nil {
		text = *d.Content
		a.text.WriteString(text)
	}
	for _, tc := range d.ToolCalls {
		i := slices.IndexFunc(a.calls, func(c schemas.ChatAssistantMessageToolCall) bool {
			if tc.ID != nil && *tc.ID != "" && c.ID != nil && *c.ID != "" {
				return *c.ID == *tc.ID
			}
			return c.Index == tc.Index
		})
		if i < 0 {
			a.calls = append(a.calls, schemas.ChatAssistantMessageToolCall{Index: tc.Index, Type: tc.Type, ID: tc.ID})
			i = len(a.calls) - 1
		}
		c := &a.calls[i]
		if c.ID == nil || *c.ID == "" {
			c.ID = tc.ID
		}
		if tc.Function.Name != nil && *tc.Function.Name != "" {
			c.Function.Name = tc.Function.Name
		}
		c.Function.Arguments += tc.Function.Arguments
		if len(tc.ExtraContent) > 0 {
			c.ExtraContent = tc.ExtraContent
		}
	}
	return text
}

func (a *streamAccumulator) message() *schemas.ChatMessage {
	text := a.text.String()
	m := &schemas.ChatMessage{Role: schemas.ChatMessageRoleAssistant, Content: &schemas.ChatMessageContent{ContentStr: &text}}
	if len(a.calls) > 0 {
		m.ChatAssistantMessage = &schemas.ChatAssistantMessage{ToolCalls: a.calls}
	}
	return m
}
