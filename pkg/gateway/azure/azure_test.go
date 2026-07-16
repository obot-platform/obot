package azure

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/system"
)

func TestIsProvider(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{system.AzureModelProvider, true},
		{system.AzureEntraModelProvider, true},
		{system.OpenAIModelProvider, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsProvider(tt.name); got != tt.want {
				t.Fatalf("IsProvider(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		creds    map[string]string
		dialect  nanobottypes.Dialect
		want     string
		wantErr  string
	}{
		{
			name:     "API key OpenAI",
			provider: system.AzureModelProvider,
			creds:    map[string]string{EndpointEnv: "https://resource.services.ai.azure.com/"},
			dialect:  nanobottypes.DialectOpenAIResponses,
			want:     "https://resource.services.ai.azure.com/openai/v1",
		},
		{
			name:     "Entra Anthropic preserves endpoint path",
			provider: system.AzureEntraModelProvider,
			creds:    map[string]string{EntraEndpointEnv: "https://resource.services.ai.azure.com/base/"},
			dialect:  nanobottypes.DialectAnthropicMessages,
			want:     "https://resource.services.ai.azure.com/base/anthropic/v1",
		},
		{
			name:     "recognized API suffix is replaced",
			provider: system.AzureModelProvider,
			creds:    map[string]string{EndpointEnv: "https://resource.openai.azure.com/openai/v1"},
			dialect:  nanobottypes.DialectAnthropicMessages,
			want:     "https://resource.openai.azure.com/anthropic/v1",
		},
		{name: "missing endpoint", provider: system.AzureModelProvider, dialect: nanobottypes.DialectOpenAIResponses, wantErr: EndpointEnv},
		{name: "invalid endpoint", provider: system.AzureModelProvider, creds: map[string]string{EndpointEnv: "not-a-url"}, dialect: nanobottypes.DialectOpenAIResponses, wantErr: "invalid Azure endpoint"},
		{name: "insecure endpoint", provider: system.AzureModelProvider, creds: map[string]string{EndpointEnv: "http://resource.openai.azure.com"}, dialect: nanobottypes.DialectOpenAIResponses, wantErr: "HTTPS"},
		{name: "non-Azure endpoint", provider: system.AzureModelProvider, creds: map[string]string{EndpointEnv: "https://example.com"}, dialect: nanobottypes.DialectOpenAIResponses, wantErr: "not a recognized Azure endpoint"},
		{name: "unsupported dialect", provider: system.AzureModelProvider, creds: map[string]string{EndpointEnv: "https://resource.openai.azure.com"}, dialect: nanobottypes.DialectOpenAIChatCompletions, wantErr: "unsupported Azure model dialect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseURL(tt.provider, tt.creds, tt.dialect)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != tt.want {
				t.Fatalf("URL = %q, want %q", got.String(), tt.want)
			}
		})
	}
}

func TestTransportMissingCredentials(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		creds    map[string]string
		want     string
	}{
		{name: "API key", provider: system.AzureModelProvider, want: APIKeyEnv},
		{name: "tenant", provider: system.AzureEntraModelProvider, want: TenantIDEnv},
		{name: "client ID", provider: system.AzureEntraModelProvider, creds: map[string]string{TenantIDEnv: "tenant"}, want: ClientIDEnv},
		{name: "client secret", provider: system.AzureEntraModelProvider, creds: map[string]string{TenantIDEnv: "tenant", ClientIDEnv: "client"}, want: ClientSecretEnv},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Transport(tt.provider, tt.creds, new(EntraCredentialCache))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestTransportReusesEntraCredential(t *testing.T) {
	cache := new(EntraCredentialCache)
	credentials := map[string]string{
		TenantIDEnv:     "tenant",
		ClientIDEnv:     "client",
		ClientSecretEnv: "secret",
	}

	firstCredential, err := cache.get(credentials)
	if err != nil {
		t.Fatal(err)
	}
	secondCredential, err := cache.get(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if secondCredential != firstCredential {
		t.Fatal("expected the Azure Entra credential to be reused")
	}

	credentials[ClientSecretEnv] = "rotated-secret"
	thirdCredential, err := cache.get(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if thirdCredential == firstCredential {
		t.Fatal("expected credential rotation to replace the cached Azure Entra credential")
	}

	otherCredential, err := new(EntraCredentialCache).get(credentials)
	if err != nil {
		t.Fatal(err)
	}
	if otherCredential == thirdCredential {
		t.Fatal("expected separate caches not to share Azure Entra credentials")
	}
}

func TestTransportRequiresEntraCredentialCache(t *testing.T) {
	_, err := Transport(system.AzureEntraModelProvider, map[string]string{
		TenantIDEnv:     "tenant",
		ClientIDEnv:     "client",
		ClientSecretEnv: "secret",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "credential cache is required") {
		t.Fatalf("error = %v, want missing cache error", err)
	}
}

func TestAPIKeyTransportHeaders(t *testing.T) {
	capture := &captureTransport{}
	req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
	req.Header.Set("Authorization", "Bearer incoming")
	req.Header.Set("api-key", "incoming-key")
	req.Header.Set("X-Api-Key", "incoming-x-key")
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	req.Header.Set("anthropic-version", "client-version")

	_, err := (apiKeyTransport{key: "azure-key", next: capture}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := capture.req.Header.Get("Authorization"); got != "Bearer azure-key" {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
	if got := capture.req.Header.Get("api-key"); got != "" {
		t.Fatalf("api-key = %q, want empty", got)
	}
	if got := capture.req.Header.Get("X-Api-Key"); got != "" {
		t.Fatalf("X-Api-Key = %q, want empty", got)
	}
	if got := capture.req.Header.Get("X-Forwarded-For"); got != "" {
		t.Fatalf("X-Forwarded-For = %q, want empty", got)
	}
	if got := capture.req.Header.Get("anthropic-version"); got != "client-version" {
		t.Fatalf("anthropic-version = %q, want client-version", got)
	}
}

func TestEntraTransportToken(t *testing.T) {
	credential := &tokenCredential{token: azcore.AccessToken{Token: "entra-token", ExpiresOn: time.Now().Add(time.Hour)}}
	capture := &captureTransport{}
	req := httptest.NewRequest(http.MethodPost, "https://example.com/responses", nil)
	req.Header.Set("Authorization", "Bearer incoming")
	req.Header.Set("api-key", "incoming-key")
	req.Header.Set("X-Api-Key", "incoming-x-key")

	_, err := (entraTransport{credential: credential, next: capture}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if got := credential.scopes; len(got) != 1 || got[0] != EntraScope {
		t.Fatalf("scopes = %v, want [%s]", got, EntraScope)
	}
	if got := capture.req.Header.Get("Authorization"); got != "Bearer entra-token" {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
	if got := capture.req.Header.Get("api-key"); got != "" {
		t.Fatalf("api-key = %q, want empty", got)
	}
	if got := capture.req.Header.Get("X-Api-Key"); got != "" {
		t.Fatalf("X-Api-Key = %q, want empty", got)
	}
}

func TestEntraTransportTokenError(t *testing.T) {
	want := errors.New("token failed")
	_, err := (entraTransport{credential: &tokenCredential{err: want}, next: &captureTransport{}}).RoundTrip(httptest.NewRequest(http.MethodPost, "https://example.com", nil))
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapping %v", err, want)
	}
}

type tokenCredential struct {
	token  azcore.AccessToken
	err    error
	scopes []string
}

func (c *tokenCredential) GetToken(_ context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.scopes = options.Scopes
	return c.token, c.err
}

type captureTransport struct {
	req *http.Request
}

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
}
