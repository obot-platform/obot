package messagepolicy

import (
	"context"

	"github.com/maximhq/bifrost/core/schemas"
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

// NewHelper returns a judge that calls the default model aliases' models through the LLM
// gateway.
func NewHelper(client kclient.Client, llmGateway completionEngine) *Helper {
	return &Helper{client: client, llmGateway: llmGateway}
}
