package messagepolicy

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/llmengine"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper evaluates natural-language policies with an LLM judge: the LLM judge guardrail,
// which message policies became.
type Helper struct {
	client     kclient.Client
	llmGateway *llmengine.Engine
}

// ApplicablePolicy pairs a policy's ID with its manifest.
type ApplicablePolicy struct {
	ID       string
	Manifest types.MessagePolicyManifest
}

// NewHelper returns a judge that calls the default model aliases' models through the LLM
// gateway.
func NewHelper(client kclient.Client, llmGateway *llmengine.Engine) *Helper {
	return &Helper{client: client, llmGateway: llmGateway}
}
