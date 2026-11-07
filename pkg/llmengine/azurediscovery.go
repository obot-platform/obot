package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	providerUtils "github.com/maximhq/bifrost/core/providers/utils"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

// Resolved credential and proxy values make cache identity directly comparable.
type azureDiscoveryCacheKey struct {
	Account       llmcoreclient.AzureAccount
	Endpoint      string
	ClientID      string
	ClientSecret  string
	TenantID      string
	ProxyType     schemas.ProxyType
	ProxyURL      string
	ProxyUsername string
	ProxyPassword string
	ProxyCA       string
	NoProxy       string
	SkipTLSVerify bool
}

type azureDiscoveryEntry struct {
	cacheKey   azureDiscoveryCacheKey
	credential azcore.TokenCredential
	client     *http.Client
}

type azureDiscovery struct {
	mu      sync.Mutex
	entries map[string]*azureDiscoveryEntry

	// Injected only by tests; production always uses the fixed ARM host.
	client     *http.Client
	credential func(schemas.Key) (azcore.TokenCredential, error)
}

func validateAzureDiscovery(key schemas.Key, account llmcoreclient.AzureAccount) error {
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "azure_subscription_id", value: account.SubscriptionID},
		{name: "azure_resource_group", value: account.ResourceGroup},
		{name: "azure_account_name", value: account.AccountName},
	} {
		if field.value == "" || field.value == "." || field.value == ".." || strings.ContainsAny(field.value, "/\\?#%") || strings.IndexFunc(field.value, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s must be a nonempty Azure account field without URL separators", field.name)
		}
	}

	c := key.AzureKeyConfig
	if c == nil || c.ClientID == nil || c.ClientSecret == nil || c.TenantID == nil || c.ClientID.GetValue() == "" || c.ClientSecret.GetValue() == "" || c.TenantID.GetValue() == "" || key.Value.GetValue() != "" {
		return fmt.Errorf("azure discovery requires explicit Entra credentials")
	}

	u, err := url.Parse(c.Endpoint.GetValue())
	if err != nil || u.Scheme != "https" || (!strings.HasSuffix(u.Hostname(), ".openai.azure.com") && !strings.HasSuffix(u.Hostname(), ".services.ai.azure.com") && !strings.HasSuffix(u.Hostname(), ".cognitiveservices.azure.com")) {
		return fmt.Errorf("azure discovery requires an Azure public cloud endpoint")
	}

	return nil
}

func (*azureDiscovery) snapshot(cfg llmcoreclient.Config, provider schemas.ModelProvider) map[string][]string {
	out := map[string][]string{}

	for _, key := range cfg.Keys[provider] {
		account := cfg.AzureAccounts[provider][key.ID]
		if account == (llmcoreclient.AzureAccount{}) {
			continue
		}

		// Presence selects automatic mode even before the first success; never backfill offerings.
		out[key.ID] = nil
		if snapshot, ok := cfg.AzureDiscoveries[provider][key.ID]; ok && snapshot.Fingerprint == llmcoreclient.AzureDiscoveryFingerprint(key, account) {
			out[key.ID] = snapshot.Names
		}
	}

	return out
}

// RefreshAzureDeployments is called only by the leader-elected model-sync controller.
func (e *Engine) RefreshAzureDeployments(ctx context.Context) error {
	cfg, err := e.config(ctx)
	if err != nil {
		return err
	}

	var failures []error

	for provider := range cfg.Providers {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isAzureProvider(cfg, provider) {
			failures = append(failures, e.discovery.refresh(ctx, cfg, provider, e.store))
		}
	}

	return errors.Join(failures...)
}

func (d *azureDiscovery) refresh(ctx context.Context, cfg llmcoreclient.Config, provider schemas.ModelProvider, store *llmcoreclient.Client) error {
	current, version, err := store.GetProviderWithVersion(ctx, string(provider))
	if err != nil {
		return err
	}
	// Recheck the provider type from the same version used for publication.
	if !isAzureProvider(llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{provider: current.Config}}, provider) {
		return nil
	}

	proxy := llmcore.ProviderProxy(current.Config, cfg.Settings["proxy"])
	snapshots := map[string]llmcoreclient.AzureDiscoverySnapshot{}
	keep := map[string]struct{}{}
	var failures []error

	for _, key := range current.Keys {
		if err := ctx.Err(); err != nil {
			return err
		}

		account := current.AzureAccounts[key.ID]
		if account == (llmcoreclient.AzureAccount{}) || (key.Enabled != nil && !*key.Enabled) {
			continue
		}

		keep[string(provider)+"/"+key.ID] = struct{}{}

		entry, err := d.keyClient(provider, key, account, proxy)
		if err != nil {
			failures = append(failures, err)
			continue
		}

		metadata := map[string]string{}
		names, err := entry.list(ctx, account.ResourceID(), metadata)
		if err != nil {
			failures = append(failures, err)
			continue
		}

		// ARM page order is not identity; canonicalize persisted deployment names.
		slices.Sort(names)
		names = slices.Compact(names)
		snapshots[key.ID] = llmcoreclient.AzureDiscoverySnapshot{
			Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account),
			Names:       names,
			BaseModels:  metadata,
		}
	}

	d.mu.Lock()
	for id := range d.entries {
		if _, exists := keep[id]; strings.HasPrefix(id, string(provider)+"/") && !exists {
			d.entries[id].client.CloseIdleConnections()
			delete(d.entries, id)
		}
	}
	d.mu.Unlock()

	// A conflict retries the whole controller pass, including ARM, rather than rebasing stale results.
	failures = append(failures, store.SaveAzureDiscoveries(ctx, string(provider), version, snapshots))

	return errors.Join(failures...)
}

func (d *azureDiscovery) keyClient(provider schemas.ModelProvider, key schemas.Key, account llmcoreclient.AzureAccount, proxy *schemas.ProxyConfig) (*azureDiscoveryEntry, error) {
	if err := validateAzureDiscovery(key, account); err != nil {
		return nil, err
	}

	c := key.AzureKeyConfig
	cacheKey := azureDiscoveryCacheKey{
		Account:      account,
		Endpoint:     c.Endpoint.GetValue(),
		ClientID:     c.ClientID.GetValue(),
		ClientSecret: c.ClientSecret.GetValue(),
		TenantID:     c.TenantID.GetValue(),
	}
	if proxy != nil {
		cacheKey.ProxyType, cacheKey.ProxyURL = proxy.Type, proxy.URL.GetValue()
		cacheKey.ProxyUsername, cacheKey.ProxyPassword = proxy.Username.GetValue(), proxy.Password.GetValue()
		cacheKey.ProxyCA, cacheKey.NoProxy, cacheKey.SkipTLSVerify = proxy.CACertPEM.GetValue(), proxy.NoProxy, proxy.SkipTLSVerify
	}
	cacheID := string(provider) + "/" + key.ID

	// Credential construction performs no network I/O; only cache access holds the mutex.
	d.mu.Lock()
	defer d.mu.Unlock()

	if entry := d.entries[cacheID]; entry != nil && entry.cacheKey == cacheKey {
		return entry, nil
	}

	client := d.client
	if client == nil {
		proxyFunc, tlsConfig, err := providerUtils.NetHTTPProxy(proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid Azure discovery proxy configuration")
		}

		transport := http.DefaultTransport.(*http.Transport).Clone()
		if proxyFunc != nil {
			transport.Proxy = proxyFunc
		}
		if tlsConfig != nil {
			transport.TLSClientConfig = tlsConfig
		}

		client = &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}

	factory := d.credential
	if factory == nil {
		factory = func(k schemas.Key) (azcore.TokenCredential, error) {
			c := k.AzureKeyConfig

			return azidentity.NewClientSecretCredential(c.TenantID.GetValue(), c.ClientID.GetValue(), c.ClientSecret.GetValue(), &azidentity.ClientSecretCredentialOptions{
				Transport: client,
			})
		}
	}

	credential, err := factory(key)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Azure discovery credentials")
	}

	if d.entries == nil {
		d.entries = map[string]*azureDiscoveryEntry{}
	}

	if previous := d.entries[cacheID]; previous != nil {
		previous.client.CloseIdleConnections()
	}
	entry := &azureDiscoveryEntry{cacheKey: cacheKey, credential: credential, client: client}
	d.entries[cacheID] = entry

	return entry, nil
}

func (entry *azureDiscoveryEntry) list(ctx context.Context, id string, metadata map[string]string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	token, err := entry.credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	if err != nil {
		return nil, fmt.Errorf("azure deployment discovery authentication failed")
	}

	next := "https://management.azure.com" + id + "/deployments?api-version=2024-10-01"
	names := []string{}
	visited := map[string]struct{}{}

	for next != "" {
		_, seen := visited[next]
		u, err := url.Parse(next)
		if err != nil || u.Scheme != "https" || u.Host != "management.azure.com" || u.User != nil || !strings.EqualFold(u.Path, id+"/deployments") || seen {
			return nil, fmt.Errorf("invalid Azure deployment pagination URL")
		}

		visited[next] = struct{}{}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+token.Token)
		resp, err := entry.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("azure deployment discovery request failed")
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()

			return nil, fmt.Errorf("azure deployment discovery returned HTTP %d; verify resource ID and deployment-read permission", resp.StatusCode)
		}

		var page struct {
			Value []struct {
				Name       string
				Properties struct {
					ProvisioningState string
					Model             struct{ Name string }
				}
			}
			NextLink string
		}

		err = json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 8<<20)).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("invalid Azure deployment discovery response")
		}

		for _, deployment := range page.Value {
			if deployment.Name != "" && deployment.Properties.ProvisioningState == "Succeeded" {
				names = append(names, deployment.Name)
				if metadata != nil {
					metadata[deployment.Name] = deployment.Properties.Model.Name
				}
			}
		}

		next = page.NextLink
	}

	return names, nil
}

func keyViews(keys []schemas.Key, accounts map[string]llmcoreclient.AzureAccount) []Key {
	out := make([]Key, 0, len(keys))

	for _, key := range keys {
		out = append(out, Key{Key: key, AzureAccount: accounts[key.ID]})
	}

	return out
}

func providerViewWithIDs(name schemas.ModelProvider, pc schemas.ProviderConfig, keys []schemas.Key, accounts map[string]llmcoreclient.AzureAccount) ProviderResponse {
	view := providerView(name, pc, keys)
	view.Keys = keyViews(view.ProviderConfig.Keys, accounts)

	return view
}
