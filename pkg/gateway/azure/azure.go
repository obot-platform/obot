package azure

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	EndpointEnv      = "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT"
	APIKeyEnv        = "OBOT_AZURE_MODEL_PROVIDER_API_KEY"
	EntraEndpointEnv = "OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT"
	ClientIDEnv      = "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID"
	ClientSecretEnv  = "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET"
	TenantIDEnv      = "OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID"

	EntraScope = "https://ai.azure.com/.default"
)

var endpointHostSuffixes = []string{
	".openai.azure.com",
	".cognitiveservices.azure.com",
	".services.ai.azure.com",
	".models.ai.azure.com",
}

func IsProvider(providerName string) bool {
	return providerName == system.AzureModelProvider || providerName == system.AzureEntraModelProvider
}

func BaseURL(providerName string, credentials map[string]string, dialect nanobottypes.Dialect) (url.URL, error) {
	endpointEnv := EndpointEnv
	if providerName == system.AzureEntraModelProvider {
		endpointEnv = EntraEndpointEnv
	} else if providerName != system.AzureModelProvider {
		return url.URL{}, fmt.Errorf("unsupported Azure model provider %q", providerName)
	}

	endpoint := credentials[endpointEnv]
	if endpoint == "" {
		return url.URL{}, fmt.Errorf("missing %s for Azure model provider", endpointEnv)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return url.URL{}, fmt.Errorf("invalid Azure endpoint: %w", err)
	}
	if err := validateEndpoint(u); err != nil {
		return url.URL{}, fmt.Errorf("invalid Azure endpoint %q: %w", endpoint, err)
	}

	basePath := strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/openai/v1", "/anthropic/v1"} {
		basePath = strings.TrimSuffix(basePath, suffix)
	}
	switch dialect {
	case nanobottypes.DialectOpenAIResponses:
		u.Path = basePath + "/openai/v1"
	case nanobottypes.DialectAnthropicMessages:
		u.Path = basePath + "/anthropic/v1"
	default:
		return url.URL{}, fmt.Errorf("unsupported Azure model dialect %q", dialect)
	}
	return *u, nil
}

// EntraCredentialCache preserves the Azure SDK credential between requests so
// its internal access-token cache can be reused instead of fetching a new token
// for every transport created by the gateway.
//
// The cache holds the most recently used Entra configuration. If any value
// changes (for example, when a client secret is rotated), get replaces the SDK
// credential. The mutex makes that comparison and replacement atomic when
// transports are created concurrently.
type EntraCredentialCache struct {
	mu           sync.Mutex
	tenantID     string
	clientID     string
	clientSecret string
	credential   azcore.TokenCredential
}

func Transport(providerName string, credentials map[string]string, entraCredentials *EntraCredentialCache) (http.RoundTripper, error) {
	switch providerName {
	case system.AzureModelProvider:
		key := credentials[APIKeyEnv]
		if key == "" {
			return nil, fmt.Errorf("missing %s for Azure model provider", APIKeyEnv)
		}
		return apiKeyTransport{key: key, next: http.DefaultTransport}, nil
	case system.AzureEntraModelProvider:
		for _, name := range []string{TenantIDEnv, ClientIDEnv, ClientSecretEnv} {
			if credentials[name] == "" {
				return nil, fmt.Errorf("missing %s for Azure Entra model provider", name)
			}
		}
		if entraCredentials == nil {
			return nil, fmt.Errorf("credential cache is required")
		}
		credential, err := entraCredentials.get(credentials)
		if err != nil {
			return nil, fmt.Errorf("create Azure Entra credential: %w", err)
		}
		return entraTransport{credential: credential, next: http.DefaultTransport}, nil
	default:
		return nil, fmt.Errorf("unsupported Azure model provider %q", providerName)
	}
}

func (c *EntraCredentialCache) get(credentials map[string]string) (azcore.TokenCredential, error) {
	tenantID := credentials[TenantIDEnv]
	clientID := credentials[ClientIDEnv]
	clientSecret := credentials[ClientSecretEnv]
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.credential != nil && c.tenantID == tenantID && c.clientID == clientID && c.clientSecret == clientSecret {
		return c.credential, nil
	}

	credential, err := azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
	if err != nil {
		return nil, err
	}
	c.tenantID = tenantID
	c.clientID = clientID
	c.clientSecret = clientSecret
	c.credential = credential
	return credential, nil
}

type apiKeyTransport struct {
	key  string
	next http.RoundTripper
}

func (t apiKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTripWithBearerToken(req, t.key, t.next)
}

type entraTransport struct {
	credential azcore.TokenCredential
	next       http.RoundTripper
}

func (t entraTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.credential.GetToken(req.Context(), policy.TokenRequestOptions{Scopes: []string{EntraScope}})
	if err != nil {
		return nil, fmt.Errorf("get Azure Entra token: %w", err)
	}
	return roundTripWithBearerToken(req, token.Token, t.next)
}

func roundTripWithBearerToken(req *http.Request, token string, next http.RoundTripper) (*http.Response, error) {
	stripProxyAndAPIKeyHeaders(req.Header)
	req.Header.Set("Authorization", "Bearer "+token)
	return next.RoundTrip(req)
}

func validateEndpoint(u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("endpoint must use HTTPS")
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint must include a host")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("endpoint must not contain user info, a query string, or a fragment")
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range endpointHostSuffixes {
		if strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return fmt.Errorf("host %q is not a recognized Azure endpoint", host)
}

func stripProxyAndAPIKeyHeaders(header http.Header) {
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "X-Api-Key", "api-key"} {
		header.Del(name)
	}
}
