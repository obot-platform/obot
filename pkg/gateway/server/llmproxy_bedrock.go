package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/bedrock"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (s *Server) newAWSBedrockLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   bedrockMantleProviderBackend{providerName: system.AmazonBedrockModelProvider},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (s *Server) newAWSBedrockAPIKeyLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   bedrockMantleProviderBackend{providerName: system.AmazonBedrockAPIKeyModelProvider, apiKey: true},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type bedrockMantleProviderBackend struct {
	providerName string
	apiKey       bool
}

func (b bedrockMantleProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b bedrockMantleProviderBackend) upstreamURL(req *http.Request, credEnv map[string]string) (url.URL, nanobottypes.Dialect, error) {
	dialect, err := resolveBedrockRouteDialect(req)
	if err != nil {
		return url.URL{}, "", types2.NewErrBadRequest("failed to determine Bedrock dialect: %v", err)
	}
	if isModelsListRequest(req) {
		u, err := bedrock.RootURL(b.resolvedProviderName(), credEnv)
		return u, dialect, err
	}
	u, err := bedrock.BaseURL(b.resolvedProviderName(), credEnv, dialect)
	return u, dialect, err
}

func (b bedrockMantleProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	return bedrock.Transport(b.resolvedProviderName(), credEnv, http.DefaultTransport)
}

func (b bedrockMantleProviderBackend) resolvedProviderName() string {
	if b.providerName != "" {
		return b.providerName
	}
	if b.apiKey {
		return system.AmazonBedrockAPIKeyModelProvider
	}
	return system.AmazonBedrockModelProvider
}

// resolveBedrockRouteDialect normalizes optional Mantle dialect prefixes and
// determines the dialect from the protocol endpoint when no prefix is present.
func resolveBedrockRouteDialect(req *http.Request) (nanobottypes.Dialect, error) {
	reqPath := strings.Trim(req.PathValue("path"), "/")
	var explicit nanobottypes.Dialect
	switch {
	case strings.HasPrefix(reqPath, "anthropic/"):
		explicit = nanobottypes.DialectAnthropicMessages
		reqPath = strings.TrimPrefix(reqPath, "anthropic/")
		req.SetPathValue("path", reqPath)
	case strings.HasPrefix(reqPath, "openai/"):
		explicit = nanobottypes.DialectOpenAIResponses
		reqPath = strings.TrimPrefix(reqPath, "openai/")
		req.SetPathValue("path", reqPath)
	}

	endpoint := strings.TrimPrefix(reqPath, "v1/")
	var inferred nanobottypes.Dialect
	switch {
	case endpoint == "models":
		return explicit, nil
	case endpoint == "messages" || strings.HasPrefix(endpoint, "messages/"):
		inferred = nanobottypes.DialectAnthropicMessages
	case endpoint == "responses" || strings.HasPrefix(endpoint, "responses/"):
		inferred = nanobottypes.DialectOpenAIResponses
	default:
		return "", fmt.Errorf("unsupported Bedrock Mantle path %q", reqPath)
	}

	if explicit != "" && explicit != inferred {
		return "", fmt.Errorf("Bedrock Mantle path prefix conflicts with endpoint %q", reqPath)
	}
	return inferred, nil
}
