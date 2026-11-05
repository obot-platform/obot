package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/azure"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type azureProviderBackend struct {
	providerName    string
	entraCredential azure.EntraCredentialCache
}

func (s *Server) newAzureLLMProviderProxy(providerName string) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   &azureProviderBackend{providerName: providerName},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (b *azureProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b *azureProviderBackend) upstreamURL(req *http.Request, credEnv map[string]string) (url.URL, llmtypes.Dialect, error) {
	dialect, err := resolveAzureRouteDialect(req)
	if err != nil {
		return url.URL{}, "", types2.NewErrBadRequest("failed to determine Azure dialect: %v", err)
	}
	u, err := azure.BaseURL(b.providerName, credEnv, dialect)
	return u, dialect, err
}

func (b *azureProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	return azure.Transport(b.providerName, credEnv, &b.entraCredential)
}

func resolveAzureRouteDialect(req *http.Request) (llmtypes.Dialect, error) {
	reqPath := strings.Trim(req.PathValue("path"), "/")
	if reqPath == "openai/v1/models" {
		req.SetPathValue("path", "v1/models")
		return llmtypes.DialectOpenAIResponses, nil
	}

	endpoint := strings.TrimPrefix(reqPath, "v1/")
	switch {
	case endpoint == "messages" || strings.HasPrefix(endpoint, "messages/"):
		return llmtypes.DialectAnthropicMessages, nil
	case endpoint == "responses" || strings.HasPrefix(endpoint, "responses/"):
		return llmtypes.DialectOpenAIResponses, nil
	case endpoint == "models":
		return llmtypes.DialectOpenAIResponses, nil
	default:
		return "", fmt.Errorf("unsupported Azure model path %q", req.PathValue("path"))
	}
}
