package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	"github.com/stretchr/testify/require"
)

func TestAzureDiscoveryEffectiveProxy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider *schemas.ProxyConfig
		global   string
		want     string
		bypass   bool
	}{
		{
			name:     "provider overrides global",
			provider: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://provider.invalid:8080")},
			global:   `{"enabled":true,"enable_for_inference":true,"url":"http://global.invalid:8080"}`,
			want:     "http://provider.invalid:8080",
		},
		{
			name:     "empty provider inherits global authentication",
			provider: &schemas.ProxyConfig{},
			global:   `{"enabled":true,"enable_for_inference":true,"url":"http://global.invalid:8080","username":"user","password":"password"}`,
			want:     "http://user:password@global.invalid:8080",
		},
		{
			name:     "socks proxy",
			provider: &schemas.ProxyConfig{Type: schemas.Socks5Proxy, URL: schemas.NewSecretVar("socks5://provider.invalid:1080")},
			want:     "socks5://provider.invalid:1080",
		},
		{
			name:   "global bypass rules",
			global: `{"enabled":true,"enable_for_inference":true,"url":"http://global.invalid:8080","no_proxy":"management.azure.com"}`,
			want:   "http://global.invalid:8080",
			bypass: true,
		},
		{
			name:   "global disabled for inference",
			global: `{"enabled":true,"enable_for_inference":false,"url":"http://global.invalid:8080"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy", "NO_PROXY", "no_proxy"} {
				t.Setenv(name, "")
			}

			key := azureTestKey("key")
			account := azureTestProvider(key).AzureAccounts[key.ID]
			proxy := llmcore.ProviderProxy(schemas.ProviderConfig{ProxyConfig: tc.provider}, json.RawMessage(tc.global))
			discovery := azureDiscovery{}
			entry, err := discovery.keyClient(schemas.Azure, key, account, proxy)
			require.NoError(t, err)
			t.Cleanup(entry.client.CloseIdleConnections)

			transport := entry.client.Transport.(*http.Transport)

			for _, host := range []string{"management.azure.com", "login.microsoftonline.com"} {
				request, err := http.NewRequest(http.MethodGet, "https://"+host+"/", nil)
				require.NoError(t, err)
				proxyURL, err := transport.Proxy(request)
				require.NoError(t, err)
				if tc.want == "" || (tc.bypass && host == "management.azure.com") {
					require.Nil(t, proxyURL)
				} else {
					require.Equal(t, tc.want, proxyURL.String())
				}
			}

			again, err := discovery.keyClient(schemas.Azure, key, account, proxy)
			require.NoError(t, err)
			require.Same(t, entry, again, "unchanged configuration must reuse its client and credential")
			request, _ := http.NewRequest(http.MethodGet, "https://management.azure.com/", nil)
			require.ErrorIs(t, entry.client.CheckRedirect(request, nil), http.ErrUseLastResponse)
		})
	}
}

func TestAzureProxyUsedForARMAndEntra(t *testing.T) {
	var mu sync.Mutex
	hosts := map[string]int{}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hosts[r.Host]++
		mu.Unlock()

		require.Equal(t, http.MethodConnect, r.Method)
		require.Equal(t, "Basic dXNlcjpwYXNzd29yZA==", r.Header.Get("Proxy-Authorization"))
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	t.Cleanup(proxy.Close)

	discovery := azureDiscovery{}
	key := azureTestKey("key")
	account := azureTestProvider(key).AzureAccounts[key.ID]
	configuration := &schemas.ProxyConfig{
		Type:     schemas.HTTPProxy,
		URL:      schemas.NewSecretVar(proxy.URL),
		Username: schemas.NewSecretVar("user"),
		Password: schemas.NewSecretVar("password"),
	}
	entry, err := discovery.keyClient(schemas.Azure, key, account, configuration)
	require.NoError(t, err)
	t.Cleanup(entry.client.CloseIdleConnections)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = entry.credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	require.Error(t, err)

	// The real SDK and the ARM request both reach the rejecting local proxy, never Azure.
	arm := azureDiscoveryEntry{credential: discoveryToken{t: t}, client: entry.client}
	_, err = arm.list(t.Context(), account.ResourceID(), nil)
	require.ErrorContains(t, err, "azure deployment discovery request failed")

	mu.Lock()
	require.Positive(t, hosts["management.azure.com:443"])
	require.Positive(t, hosts["login.microsoftonline.com:443"])
	mu.Unlock()

	configuration.Password = schemas.NewSecretVar("changed")
	replacement, err := discovery.keyClient(schemas.Azure, key, account, configuration)
	require.NoError(t, err)
	t.Cleanup(replacement.client.CloseIdleConnections)
	require.NotSame(t, entry, replacement, "proxy changes must replace the credential's bound HTTP client")
	require.NotSame(t, entry.credential, replacement.credential)
}

func TestAzureDiscoveryProxyChangesAndErrors(t *testing.T) {
	discovery := azureDiscovery{}
	key := azureTestKey("key")
	account := azureTestProvider(key).AzureAccounts[key.ID]
	proxy := &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://first.invalid:8080")}
	first, err := discovery.keyClient(schemas.Azure, key, account, proxy)
	require.NoError(t, err)
	t.Cleanup(first.client.CloseIdleConnections)

	proxy.URL = schemas.NewSecretVar("http://second.invalid:8080")
	second, err := discovery.keyClient(schemas.Azure, key, account, proxy)
	require.NoError(t, err)
	t.Cleanup(second.client.CloseIdleConnections)
	require.NotSame(t, first, second)
	request, err := http.NewRequest(http.MethodGet, "https://management.azure.com/", nil)
	require.NoError(t, err)
	selected, err := second.client.Transport.(*http.Transport).Proxy(request)
	require.NoError(t, err)
	require.Equal(t, "second.invalid:8080", selected.Host)

	proxy.NoProxy = "management.azure.com"
	bypass, err := discovery.keyClient(schemas.Azure, key, account, proxy)
	require.NoError(t, err)
	t.Cleanup(bypass.client.CloseIdleConnections)
	require.NotSame(t, second, bypass)
	selected, err = bypass.client.Transport.(*http.Transport).Proxy(request)
	require.NoError(t, err)
	require.Nil(t, selected)

	proxy.URL = schemas.NewSecretVar("http://user:secret@%invalid")
	_, err = discovery.keyClient(schemas.Azure, key, account, proxy)
	require.EqualError(t, err, "invalid Azure discovery proxy configuration")
}
