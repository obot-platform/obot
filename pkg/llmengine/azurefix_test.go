package llmengine

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func azureTestKey(id string) schemas.Key {
	return schemas.Key{ID: id, Name: id, Models: schemas.WhiteList{"*"}, Weight: 1, AzureKeyConfig: &schemas.AzureKeyConfig{
		Endpoint: *schemas.NewSecretVar("https://account.openai.azure.com"),
		ClientID: schemas.NewSecretVar("client"), ClientSecret: schemas.NewSecretVar("secret"), TenantID: schemas.NewSecretVar("tenant"),
	}}
}

func azureTestProvider(keys ...schemas.Key) llmcoreclient.Provider {
	accounts := map[string]llmcoreclient.AzureAccount{}

	for _, key := range keys {
		accounts[key.ID] = llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: key.ID}
	}

	return llmcoreclient.Provider{Keys: keys, AzureAccounts: accounts}
}

func mockAzureDiscovery(t *testing.T, e *Engine, transport discoveryTransport) {
	t.Helper()

	e.discovery.mu.Lock()
	defer e.discovery.mu.Unlock()

	// Replacing a test transport also replaces clients bound to the previous transport.
	e.discovery.entries = nil
	e.discovery.credential = func(schemas.Key) (azcore.TokenCredential, error) { return discoveryToken{t: t}, nil }
	e.discovery.client = &http.Client{Transport: transport}
}

func azureResponse(status int, names ...string) *http.Response {
	entries := []string{}

	for _, name := range names {
		entries = append(entries, fmt.Sprintf(`{"name":%q,"properties":{"provisioningState":"Succeeded","model":{"name":"gpt-4o"}}}`, name))
	}

	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"value":[` + strings.Join(entries, ",") + `]}`)), Header: http.Header{}}
}

func TestAzureRefreshCannotFallThrough(t *testing.T) {
	e, ctx := startTestEngine(t)
	var calls atomic.Int32
	offerings := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"offering"}]}`))
	}))
	defer offerings.Close()

	// Keep the runtime on a local API-key endpoint. The hook's desired Entra config
	// is separate so a broken short circuit reaches a deterministic offering, never Azure.
	key := azureTestKey("key")
	runtimeKey := key
	runtimeKey.Value = *schemas.NewSecretVar("api-key")
	runtimeKey.AzureKeyConfig = &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar(offerings.URL)}
	require.NoError(t, e.store.CreateProvider(ctx, "azure", llmcoreclient.Provider{
		Keys: []schemas.Key{runtimeKey}, Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{AllowPrivateNetwork: true}},
	}))
	_, err := e.Sync(ctx)
	require.NoError(t, err)

	cfg, err := e.config(ctx)
	require.NoError(t, err)

	cfg.Keys[schemas.Azure] = []schemas.Key{key}
	cfg.AzureAccounts[schemas.Azure] = azureTestProvider(key).AzureAccounts
	mockAzureDiscovery(t, e, func(*http.Request) (*http.Response, error) { return azureResponse(http.StatusForbidden), nil })
	require.NoError(t, e.store.MutateProvider(ctx, "azure", func(provider *llmcoreclient.Provider) error {
		provider.Keys = []schemas.Key{key}
		provider.AzureAccounts = azureTestProvider(key).AzureAccounts

		return nil
	}))
	desired := context.WithValue(ctx, configContextKey{}, &cfg)
	// Exercise the explicit refresh list call without resyncing this local-only runtime.
	_, err = e.listGatewayModels(desired, schemas.Azure, true)
	require.NoError(t, err)
	require.ErrorContains(t, e.RefreshAzureDeployments(ctx), "HTTP 403")
	require.Zero(t, calls.Load(), "ARM failure must not invoke the data-plane offerings endpoint")

	// Configuration read failures must use the same terminal contract.
	broken := New()
	bctx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	_, short, hookErr := broken.availability.PreLLMHook(bctx, &schemas.BifrostRequest{ListModelsRequest: &schemas.BifrostListModelsRequest{Provider: schemas.Azure}})
	require.NoError(t, hookErr)
	require.NotNil(t, short.Error)
	require.False(t, *short.Error.AllowFallbacks)
}

func TestAzureLeaderProcessesKeysSequentiallyWithoutBlockingPublicLists(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "azure", azureTestProvider(azureTestKey("slow"), azureTestKey("healthy"))))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"fast"}]}`))
	}))
	t.Cleanup(upstream.Close)
	require.NoError(t, e.store.CreateProvider(ctx, "openai", llmcoreclient.Provider{
		Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true}},
		Keys:   []schemas.Key{{ID: "openai", Name: "openai", Value: *schemas.NewSecretVar("secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
	}))
	_, err := e.Sync(ctx)
	require.NoError(t, err)

	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	mockAzureDiscovery(t, e, func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if strings.Contains(r.URL.Path, "/accounts/slow/") {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
			return azureResponse(http.StatusForbidden), nil
		}
		return azureResponse(http.StatusOK, "healthy-deployment"), nil
	})
	done := make(chan error, 1)
	go func() { done <- e.RefreshAzureDeployments(ctx) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("leader did not start discovery")
	}

	models, err := e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	require.Empty(t, models, "public listings read snapshots without waiting for ARM")
	models, err = e.gatewayModels(ctx, schemas.OpenAI)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.EqualValues(t, 1, calls.Load(), "the second key must not start concurrently")
	close(release)
	require.ErrorContains(t, <-done, "HTTP 403")
	require.EqualValues(t, 2, calls.Load(), "one failing key must not starve the next")

	models, err = e.gatewayModels(ctx, schemas.Azure)
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "azure/healthy-deployment", models[0].ID)
}

func TestAzureMetadataIgnoresBodyIDAndPreservesMaskedEdits(t *testing.T) {
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: testKeyDNS}
	t.Cleanup(func() { net.DefaultResolver = previous })

	e, ctx := startTestEngine(t)
	provider := azureTestProvider(azureTestKey("a"), azureTestKey("b"))
	require.NoError(t, e.store.CreateProvider(ctx, "azure", provider))

	for _, bodyID := range []string{"b", "unknown"} {
		_, err := e.UpdateKey(ctx, "azure", "a", mustJSON(t, map[string]any{"id": bodyID, "name": "renamed"}))
		require.NoError(t, err)

		stored, err := e.store.GetProvider(ctx, "azure")
		require.NoError(t, err)
		require.Equal(t, provider.AzureAccounts, stored.AzureAccounts)
	}

	created, err := e.CreateKey(ctx, "azure", mustJSON(t, map[string]any{"id": "b", "name": "created", "value": "secret", "azure_key_config": map[string]any{"endpoint": "https://account.openai.azure.com"}}))
	require.NoError(t, err)
	require.NotEqual(t, "b", created.ID)
	require.Empty(t, created.AzureAccount)

	view, err := e.GetKey(ctx, "azure", "a")
	require.NoError(t, err)

	_, err = e.UpdateKey(ctx, "azure", "a", mustJSON(t, map[string]any{
		"value": view.Value, "azure_key_config": view.AzureKeyConfig, "azure_resource_group": " edited ",
	}))
	require.NoError(t, err)

	stored, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Equal(t, "secret", stored.Keys[0].AzureKeyConfig.ClientSecret.GetValue())
	require.Equal(t, llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "edited", AccountName: "a"}, stored.AzureAccounts["a"])
	require.Equal(t, provider.AzureAccounts["b"], stored.AzureAccounts["b"])

	_, err = e.UpdateKey(ctx, "azure", "a", []byte(`{"azure_subscription_id":"","azure_resource_group":" ","azure_account_name":""}`))
	require.NoError(t, err)

	stored, err = e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.NotContains(t, stored.AzureAccounts, "a")
	require.Contains(t, stored.AzureAccounts, "b")
}

func TestAzureSnapshotsSurviveFreshEngineAndStayPerKey(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "azure", azureTestProvider(azureTestKey("one"), azureTestKey("two"))))

	mockAzureDiscovery(t, e, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/accounts/one/") {
			return azureResponse(200, "first"), nil
		}

		return azureResponse(200, "second"), nil
	})

	require.NoError(t, e.RefreshAzureDeployments(ctx))

	fresh := New()
	require.NoError(t, fresh.Start(ctx, Options{Store: e.store, DataDir: e.opts.DataDir}))
	t.Cleanup(func() { require.NoError(t, fresh.Shutdown(context.Background())) })

	mockAzureDiscovery(t, fresh, func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.Path, "/accounts/one/") {
			return azureResponse(403), nil
		}

		return azureResponse(200, "new-second"), nil
	})

	require.ErrorContains(t, fresh.RefreshAzureDeployments(ctx), "HTTP 403")

	models, err := fresh.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 2)
	require.Equal(t, "azure/first", models.Models[0].ID)
	require.Equal(t, "azure/new-second", models.Models[1].ID)

	unavailable, err := fresh.UnavailableModels(ctx, []string{"azure/first", "azure/new-second", "azure/second", "openai/unknown"})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"azure/second": true}, unavailable)
	require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
		p.Keys[0].AzureKeyConfig.ClientSecret = schemas.NewSecretVar("replacement")

		return nil
	}))
	models, err = fresh.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	require.Equal(t, "azure/new-second", models.Models[0].ID)

	mockAzureDiscovery(t, fresh, func(*http.Request) (*http.Response, error) { return azureResponse(200), nil })
	require.NoError(t, fresh.RefreshAzureDeployments(ctx))
	models, err = fresh.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models.Models)

	// An empty success must survive a restart just as a nonempty success does.
	cfg, err := fresh.config(ctx)
	require.NoError(t, err)
	require.Contains(t, cfg.AzureDiscoveries[schemas.Azure], "one")
	require.Empty(t, cfg.AzureDiscoveries[schemas.Azure]["one"].Names)
}

func TestAzureInFlightSnapshotCannotFollowCredentialEdit(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "azure", azureTestProvider(azureTestKey("key"))))

	entered, release := make(chan struct{}), make(chan struct{})
	mockAzureDiscovery(t, e, func(r *http.Request) (*http.Response, error) {
		close(entered)

		select {
		case <-release:
			return azureResponse(200, "old-account-deployment"), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})

	cfg, err := e.config(ctx)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- e.discovery.refresh(ctx, cfg, schemas.Azure, e.store) }()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("ARM request did not start")
	}

	require.NoError(t, e.store.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
		p.Keys[0].AzureKeyConfig.ClientSecret = schemas.NewSecretVar("replacement")

		return nil
	}))
	close(release)
	require.ErrorContains(t, <-done, "provider changed during Azure discovery")

	current, err := e.config(ctx)
	require.NoError(t, err)
	require.Empty(t, current.AzureDiscoveries[schemas.Azure])
	require.Empty(t, e.discovery.snapshot(current, schemas.Azure)["key"])
}

func TestAzureFormerLeaderCannotOverwriteNewOwner(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "azure", azureTestProvider(azureTestKey("key"))))

	entered, release := make(chan struct{}), make(chan struct{})
	mockAzureDiscovery(t, e, func(r *http.Request) (*http.Response, error) {
		close(entered)
		select {
		case <-release:
			return azureResponse(http.StatusOK, "deleted-deployment"), nil
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
	})
	newOwner := New()
	require.NoError(t, newOwner.Start(ctx, Options{Store: e.store, DataDir: e.opts.DataDir}))
	t.Cleanup(func() { require.NoError(t, newOwner.Shutdown(context.Background())) })
	mockAzureDiscovery(t, newOwner, func(*http.Request) (*http.Response, error) {
		return azureResponse(http.StatusOK), nil
	})

	done := make(chan error, 1)
	go func() { done <- e.RefreshAzureDeployments(ctx) }()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("older discovery did not start")
	}

	require.NoError(t, newOwner.RefreshAzureDeployments(ctx))
	close(release)
	require.ErrorContains(t, <-done, "provider changed during Azure discovery")

	current, err := e.config(ctx)
	require.NoError(t, err)
	require.Contains(t, current.AzureDiscoveries[schemas.Azure], "key")
	require.Empty(t, current.AzureDiscoveries[schemas.Azure]["key"].Names)
	require.Empty(t, current.AzureDiscoveries[schemas.Azure]["key"].BaseModels)
}

func TestAzureCanceledOwnerCannotPublishEvenWhenARMIgnoresCancellation(t *testing.T) {
	e, ctx := startTestEngine(t)
	require.NoError(t, e.store.CreateProvider(ctx, "azure", azureTestProvider(azureTestKey("key"))))
	entered, release := make(chan struct{}), make(chan struct{})
	mockAzureDiscovery(t, e, func(*http.Request) (*http.Response, error) {
		close(entered)
		<-release
		return azureResponse(http.StatusOK, "late"), nil
	})
	ownerCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- e.RefreshAzureDeployments(ownerCtx) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("discovery did not start")
	}
	cancel()
	close(release)
	require.ErrorIs(t, <-done, context.Canceled)
	provider, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Empty(t, provider.AzureDiscoveries)
}
