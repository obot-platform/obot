package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/azure"
	"github.com/obot-platform/obot/pkg/system"
)

func TestAzureProviderBackend(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		provider string
		dialect  nanobottypes.Dialect
		creds    map[string]string
		wantURL  string
	}{
		{
			name:     "API key OpenAI",
			method:   http.MethodPost,
			path:     "v1/responses",
			provider: system.AzureModelProvider,
			dialect:  nanobottypes.DialectOpenAIResponses,
			creds:    map[string]string{azure.EndpointEnv: "https://resource.openai.azure.com", azure.APIKeyEnv: "key"},
			wantURL:  "https://resource.openai.azure.com/openai/v1/responses",
		},
		{
			name:     "Entra Anthropic",
			method:   http.MethodPost,
			path:     "v1/messages",
			provider: system.AzureEntraModelProvider,
			dialect:  nanobottypes.DialectAnthropicMessages,
			creds:    map[string]string{azure.EntraEndpointEnv: "https://resource.services.ai.azure.com"},
			wantURL:  "https://resource.services.ai.azure.com/anthropic/v1/messages",
		},
		{
			name:     "Entra models",
			method:   http.MethodGet,
			path:     "v1/models",
			provider: system.AzureEntraModelProvider,
			dialect:  nanobottypes.DialectOpenAIResponses,
			creds:    map[string]string{azure.EntraEndpointEnv: "https://resource.services.ai.azure.com"},
			wantURL:  "https://resource.services.ai.azure.com/openai/v1/models",
		},
		{
			name:     "Entra prefixed models",
			method:   http.MethodGet,
			path:     "openai/v1/models",
			provider: system.AzureEntraModelProvider,
			dialect:  nanobottypes.DialectOpenAIResponses,
			creds:    map[string]string{azure.EntraEndpointEnv: "https://resource.services.ai.azure.com"},
			wantURL:  "https://resource.services.ai.azure.com/openai/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := azureProviderBackend{providerName: tt.provider}
			if got := backend.modelProviderName(); got != tt.provider {
				t.Fatalf("provider = %q, want %q", got, tt.provider)
			}
			req := httptest.NewRequest(tt.method, "http://gateway.local", nil)
			req.SetPathValue("path", tt.path)
			base, dialect, err := backend.upstreamURL(req, tt.creds)
			if err != nil {
				t.Fatal(err)
			}
			if dialect != tt.dialect {
				t.Fatalf("dialect = %q, want %q", dialect, tt.dialect)
			}
			llmTransformRequest(base)(req)
			if got := req.URL.String(); got != tt.wantURL {
				t.Fatalf("URL = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

func TestAzureProviderBackendRejectsUnsupportedPathAsBadRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://gateway.local", nil)
	req.SetPathValue("path", "v1/chat/completions")

	_, _, err := (&azureProviderBackend{}).upstreamURL(req, nil)
	var httpErr *types2.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %v, want *types.ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", httpErr.Code, http.StatusBadRequest)
	}
}

func TestNewAzureLLMProviderProxy(t *testing.T) {
	s := new(Server)
	proxy := s.newAzureLLMProviderProxy(system.AzureEntraModelProvider)
	if got := proxy.backend.modelProviderName(); got != system.AzureEntraModelProvider {
		t.Fatalf("provider = %q, want %q", got, system.AzureEntraModelProvider)
	}
}

func TestResolveAzureRouteDialect(t *testing.T) {
	tests := []struct {
		path    string
		want    nanobottypes.Dialect
		wantErr bool
	}{
		{path: "messages", want: nanobottypes.DialectAnthropicMessages},
		{path: "v1/messages", want: nanobottypes.DialectAnthropicMessages},
		{path: "responses", want: nanobottypes.DialectOpenAIResponses},
		{path: "v1/responses/response-id", want: nanobottypes.DialectOpenAIResponses},
		{path: "v1/models", want: nanobottypes.DialectOpenAIResponses},
		{path: "openai/v1/models", want: nanobottypes.DialectOpenAIResponses},
		{path: "openai/v1/responses", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://gateway.local", nil)
			req.SetPathValue("path", tt.path)
			got, err := resolveAzureRouteDialect(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("dialect = %q, want %q", got, tt.want)
			}
		})
	}
}
