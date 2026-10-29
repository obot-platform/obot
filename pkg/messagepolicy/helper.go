package messagepolicy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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

// structuredCompletionEngine is implemented by gateways that accept request parameters,
// such as response_format.
type structuredCompletionEngine interface {
	ChatCompletionWithParams(context.Context, string, []schemas.ChatMessage, *schemas.ChatParameters) (*schemas.BifrostChatResponse, error)
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
