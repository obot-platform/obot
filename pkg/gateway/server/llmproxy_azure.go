package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/gateway/azure"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (s *Server) newAzureLLMProviderProxy(providerName string) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   &azureProviderBackend{providerName: providerName},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type azureProviderBackend struct {
	providerName    string
	entraCredential azure.EntraCredentialCache
}

func (b *azureProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b *azureProviderBackend) upstreamURL(req *http.Request, credEnv map[string]string) (url.URL, nanobottypes.Dialect, error) {
	dialect, err := resolveAzureRouteDialect(req)
	if err != nil {
		return url.URL{}, "", err
	}
	u, err := azure.BaseURL(b.providerName, credEnv, dialect)
	return u, dialect, err
}

func (b *azureProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string, dialect nanobottypes.Dialect) (http.RoundTripper, error) {
	return azure.Transport(b.providerName, credEnv, dialect, &b.entraCredential)
}

func resolveAzureRouteDialect(req *http.Request) (nanobottypes.Dialect, error) {
	endpoint := strings.TrimPrefix(strings.Trim(req.PathValue("path"), "/"), "v1/")
	switch {
	case endpoint == "messages" || strings.HasPrefix(endpoint, "messages/"):
		return nanobottypes.DialectAnthropicMessages, nil
	case endpoint == "responses" || strings.HasPrefix(endpoint, "responses/"):
		return nanobottypes.DialectOpenAIResponses, nil
	default:
		return "", fmt.Errorf("unsupported Azure model path %q", req.PathValue("path"))
	}
}
