package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	genericResponsesBaseURLEnv = "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL"
	genericResponsesAPIKeyEnv  = "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY"
)

func (s *Server) newGenericResponsesLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   genericResponsesProviderBackend{},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type genericResponsesProviderBackend struct{}

func (genericResponsesProviderBackend) modelProviderName() string {
	return system.GenericResponsesModelProvider
}

func (genericResponsesProviderBackend) upstreamURL(_ *http.Request, credEnv map[string]string) (url.URL, nanobottypes.Dialect, error) {
	rawURL := strings.TrimSpace(credEnv[genericResponsesBaseURLEnv])
	u, err := url.Parse(rawURL)
	if err != nil {
		return url.URL{}, "", fmt.Errorf("failed to parse Generic Responses base URL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return url.URL{}, "", fmt.Errorf("generic Responses base URL must be an absolute HTTP or HTTPS URL")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return *u, nanobottypes.DialectOpenResponses, nil
}

func (genericResponsesProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	return genericResponsesTransport{
		key:  credEnv[genericResponsesAPIKeyEnv],
		next: http.DefaultTransport,
	}, nil
}

type genericResponsesTransport struct {
	key  string
	next http.RoundTripper
}

func (g genericResponsesTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	if g.key != "" {
		req.Header.Set("Authorization", "Bearer "+g.key)
	}
	return g.next.RoundTrip(req)
}
