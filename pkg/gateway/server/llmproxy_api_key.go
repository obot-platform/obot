package server

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type apiKeyLLMProviderBackend struct {
	u            url.URL
	providerName string
}

func (a apiKeyLLMProviderBackend) modelProviderName() string {
	return a.providerName
}

func (a apiKeyLLMProviderBackend) upstreamURL(map[string]string) (url.URL, error) {
	return a.u, nil
}

func (a apiKeyLLMProviderBackend) transport(modelProvider v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	credEnvKey, err := envVarForModelProvider(modelProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get credential environment key for model provider: %w", err)
	}
	return apiKeyTransport{providerName: modelProvider.Name, key: credEnv[credEnvKey], next: http.DefaultTransport}, nil
}

type apiKeyTransport struct {
	providerName string
	key          string
	next         http.RoundTripper
}

func (a apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch a.providerName {
	case system.AnthropicModelProvider:
		req.Header.Del("Authorization")
		req.Header.Set("X-Api-Key", a.key)
	case system.OpenAIModelProvider:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.key))
		req.Header.Del("X-Api-Key")
	default:
		if bearer := req.Header.Get("Authorization"); bearer != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.key))
		} else if token := req.Header.Get("X-Api-Key"); token != "" {
			req.Header.Set("X-Api-Key", a.key)
		}
	}
	next := a.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

func (a apiKeyLLMProviderBackend) proxyModelsList(api.Context, *llmProviderProxy, *v1.ModelProvider, map[string]string) (bool, error) {
	return false, nil
}
