package server

import (
	"net/http"
	"net/url"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/gateway/bedrock"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (s *Server) newAWSBedrockLLMProviderProxy(dialect nanobottypes.Dialect) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		routeDialect:              dialect,
		backend:                   bedrockMantleProviderBackend{providerName: system.AmazonBedrockModelProvider, dialect: dialect},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (s *Server) newAWSBedrockAPIKeyLLMProviderProxy(dialect nanobottypes.Dialect) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		routeDialect:              dialect,
		backend:                   bedrockMantleProviderBackend{providerName: system.AmazonBedrockAPIKeyModelProvider, dialect: dialect, apiKey: true},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type bedrockMantleProviderBackend struct {
	providerName string
	dialect      nanobottypes.Dialect
	apiKey       bool
}

func (b bedrockMantleProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b bedrockMantleProviderBackend) upstreamURL(credEnv map[string]string) (url.URL, error) {
	return bedrock.BaseURL(b.resolvedProviderName(), credEnv, b.dialect)
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
