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
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

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
		return fmt.Errorf("Azure discovery requires explicit Entra credentials")
	}
	u, err := url.Parse(c.Endpoint.GetValue())
	if err != nil || u.Scheme != "https" || !(strings.HasSuffix(u.Hostname(), ".openai.azure.com") || strings.HasSuffix(u.Hostname(), ".services.ai.azure.com") || strings.HasSuffix(u.Hostname(), ".cognitiveservices.azure.com")) {
		return fmt.Errorf("Azure discovery requires an Azure public cloud endpoint")
	}
	return nil
}

type azureDiscoveryEntry struct {
	fingerprint [32]byte
	credential  azcore.TokenCredential
}

type azureDiscovery struct {
	mu      sync.Mutex
	entries map[string]*azureDiscoveryEntry
	// Injected only by tests; production always uses the fixed ARM host.
	client     *http.Client
	credential func(schemas.Key) (azcore.TokenCredential, error)
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

func (d *azureDiscovery) refresh(ctx context.Context, cfg llmcoreclient.Config, provider schemas.ModelProvider, store *llmcoreclient.Client) error {
	// Workers can finish after caller cancellation without blocking on result delivery.
	results := make(chan error, len(cfg.Keys[provider]))
	pending := 0
	keep := map[string]bool{}
	for _, key := range cfg.Keys[provider] {
		account := cfg.AzureAccounts[provider][key.ID]
		if account == (llmcoreclient.AzureAccount{}) || (key.Enabled != nil && !*key.Enabled) {
			continue
		}
		keep[string(provider)+"/"+key.ID] = true
		pending++
		// Each key gets the caller's whole window; a slow key cannot starve healthy keys.
		go func() {
			credential, err := d.keyCredential(provider, key, account)
			if err == nil {
				metadata := map[string]string{}
				var names []string
				names, err = d.list(ctx, credential, account.ResourceID(), metadata)
				if err == nil {
					// ARM page order is not identity; canonical names prevent redundant watch events.
					slices.Sort(names)
					names = slices.Compact(names)
					err = store.SaveAzureDiscovery(ctx, string(provider), key.ID, llmcoreclient.AzureDiscoverySnapshot{
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account), Names: names, BaseModels: metadata,
					})
				}
			}
			results <- err
		}()
	}
	d.mu.Lock()
	for id := range d.entries {
		if strings.HasPrefix(id, string(provider)+"/") && !keep[id] {
			delete(d.entries, id)
		}
	}
	d.mu.Unlock()
	var failures []error
	for range pending {
		select {
		case err := <-results:
			failures = append(failures, err)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return errors.Join(failures...)
}

func (d *azureDiscovery) keyCredential(provider schemas.ModelProvider, key schemas.Key, account llmcoreclient.AzureAccount) (azcore.TokenCredential, error) {
	if err := validateAzureDiscovery(key, account); err != nil {
		return nil, err
	}
	fingerprint := llmcoreclient.AzureDiscoveryFingerprint(key, account)
	cacheID := string(provider) + "/" + key.ID
	// Credential construction performs no network I/O; only cache access holds the mutex.
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry := d.entries[cacheID]; entry != nil && entry.fingerprint == fingerprint {
		return entry.credential, nil
	}
	factory := d.credential
	if factory == nil {
		factory = func(k schemas.Key) (azcore.TokenCredential, error) {
			c := k.AzureKeyConfig
			return azidentity.NewClientSecretCredential(c.TenantID.GetValue(), c.ClientID.GetValue(), c.ClientSecret.GetValue(), nil)
		}
	}
	credential, err := factory(key)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Azure discovery credentials")
	}
	if d.entries == nil {
		d.entries = map[string]*azureDiscoveryEntry{}
	}
	d.entries[cacheID] = &azureDiscoveryEntry{fingerprint: fingerprint, credential: credential}
	return credential, nil
}

func (d *azureDiscovery) list(ctx context.Context, credential azcore.TokenCredential, id string, metadata ...map[string]string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	token, err := credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://management.azure.com/.default"}})
	if err != nil {
		return nil, fmt.Errorf("Azure deployment discovery authentication failed")
	}
	client := d.client
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	next := "https://management.azure.com" + id + "/deployments?api-version=2024-10-01"
	names := []string{}
	visited := map[string]bool{}
	for next != "" {
		u, err := url.Parse(next)
		if err != nil || u.Scheme != "https" || u.Host != "management.azure.com" || u.User != nil || !strings.EqualFold(u.Path, id+"/deployments") || visited[next] {
			return nil, fmt.Errorf("invalid Azure deployment pagination URL")
		}
		visited[next] = true
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token.Token)
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Azure deployment discovery request failed")
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("Azure deployment discovery returned HTTP %d; verify resource ID and deployment-read permission", resp.StatusCode)
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
				if len(metadata) > 0 {
					metadata[0][deployment.Name] = deployment.Properties.Model.Name
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

// Deployment names remain the wire IDs; catalog metadata belongs to the underlying model.
func (*azureDiscovery) baseModel(cfg llmcoreclient.Config, provider schemas.ModelProvider, deployment string) string {
	for _, key := range cfg.Keys[provider] {
		snapshot, ok := cfg.AzureDiscoveries[provider][key.ID]
		if ok && (key.Enabled == nil || *key.Enabled) && snapshot.Fingerprint == llmcoreclient.AzureDiscoveryFingerprint(key, cfg.AzureAccounts[provider][key.ID]) {
			if name := snapshot.BaseModels[deployment]; name != "" {
				return name
			}
		}
	}
	return ""
}
