package llmengine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

type discoveryToken struct{ t *testing.T }

type discoveryTransport func(*http.Request) (*http.Response, error)

func (c discoveryToken) GetToken(_ context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	require.Equal(c.t, []string{"https://management.azure.com/.default"}, options.Scopes)

	return azcore.AccessToken{Token: "test-token"}, nil
}

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAzureEntraDiscovery(t *testing.T) {
	e, ctx := startTestEngine(t)
	const resourceID = "/subscriptions/sub/resourceGroups/group/providers/Microsoft.CognitiveServices/accounts/account"
	status := http.StatusOK
	body := `{"value":[{"name":"custom-deployment","properties":{"provisioningState":"Succeeded"}},{"name":"not-ready","properties":{"provisioningState":"Creating"}}]}`
	calls := 0
	credentials := 0
	e.discovery.credential = func(schemas.Key) (azcore.TokenCredential, error) {
		credentials++

		return discoveryToken{t: t}, nil
	}

	e.discovery.client = &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "management.azure.com", r.URL.Host)
		require.Equal(t, resourceID+"/deployments", r.URL.Path)
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}
	_, err := e.CreateProvider(ctx, []byte(`{"provider":"azure"}`))
	require.NoError(t, err)

	key, err := e.CreateKey(ctx, "azure", []byte(`{"name":"entra","models":["*"],"azure_subscription_id":"sub","azure_resource_group":"group","azure_account_name":"account","azure_key_config":{"endpoint":"https://account.openai.azure.com","client_id":"client","client_secret":"secret","tenant_id":"tenant"}}`))
	require.NoError(t, err)
	require.Equal(t, resourceID, key.ResourceID())

	stored, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Equal(t, resourceID, stored.AzureAccounts[key.ID].ResourceID())
	require.NoError(t, e.RefreshAzureDeployments(ctx))

	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	require.Equal(t, "azure/custom-deployment", models.Models[0].ID)

	status = http.StatusForbidden
	require.Error(t, e.RefreshAzureDeployments(ctx))

	models, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Len(t, models.Models, 1)
	require.Equal(t, 1, credentials, "reuse the Azure credential across listings")

	_, err = e.UpdateKey(ctx, "azure", key.ID, []byte(`{"azure_key_config":{"client_secret":"replacement"}}`))
	require.NoError(t, err)

	models, err = e.ListModels(ctx)
	if err == nil {
		require.Empty(t, models.Models, "changed credentials cannot reuse old discovery")
	}

	require.Equal(t, 1, credentials, "public listing does not create a credential")

	status = http.StatusOK
	body = `{"value":[]}`
	require.NoError(t, e.RefreshAzureDeployments(ctx))
	require.Equal(t, 2, credentials)
	models, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, models.Models)
	require.Greater(t, calls, 0)

	_, err = e.UpdateKey(ctx, "azure", key.ID, []byte(`{"name":"renamed"}`))
	require.NoError(t, err)

	got, err := e.GetKey(ctx, "azure", key.ID)
	require.NoError(t, err)
	require.Equal(t, resourceID, got.ResourceID())
	require.NoError(t, e.DeleteKey(ctx, "azure", key.ID))

	stored, err = e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Empty(t, stored.AzureAccounts)
}

func TestAzureDiscoveryPagination(t *testing.T) {
	const id = "/subscriptions/sub/resourceGroups/group/providers/Microsoft.CognitiveServices/accounts/account"

	for _, next := range []string{"https://management.azure.com" + id + "/deployments?page=2", "https://evil.example/deployments", "https://management.azure.com/other/deployments"} {
		t.Run(next, func(t *testing.T) {
			calls := 0
			entry := azureDiscoveryEntry{credential: discoveryToken{t: t}, client: &http.Client{Transport: discoveryTransport(func(_ *http.Request) (*http.Response, error) {
				calls++
				body := `{"value":[{"name":"second","properties":{"provisioningState":"Succeeded"}}]}`
				if calls == 1 {
					body = `{"value":[],"nextLink":"` + next + `"}`
				}

				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}}
			names, err := entry.list(t.Context(), id, nil)
			if strings.Contains(next, id) {
				require.NoError(t, err)
				require.Equal(t, []string{"second"}, names)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, calls)
			}
		})
	}
}

func TestAzureDiscoveryAccountFields(t *testing.T) {
	key := schemas.Key{AzureKeyConfig: &schemas.AzureKeyConfig{
		Endpoint:     schemas.SecretVar{Val: "https://account.openai.azure.com"},
		ClientID:     &schemas.SecretVar{Val: "client"},
		ClientSecret: &schemas.SecretVar{Val: "secret"},
		TenantID:     &schemas.SecretVar{Val: "tenant"},
	}}
	account := llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
	require.NoError(t, validateAzureDiscovery(key, account))

	for _, tc := range []struct {
		name    string
		account llmcoreclient.AzureAccount
	}{
		{
			name:    "missing subscription",
			account: llmcoreclient.AzureAccount{ResourceGroup: "group", AccountName: "account"},
		},
		{
			name:    "missing group",
			account: llmcoreclient.AzureAccount{SubscriptionID: "sub", AccountName: "account"},
		},
		{
			name:    "missing account",
			account: llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group"},
		},
		{
			name:    "path separator",
			account: llmcoreclient.AzureAccount{SubscriptionID: "sub/other", ResourceGroup: "group", AccountName: "account"},
		},
		{
			name:    "query injection",
			account: llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group?test", AccountName: "account"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Error(t, validateAzureDiscovery(key, tc.account)) })
	}

	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, []byte(`{"provider":"azure"}`))
	require.NoError(t, err)

	_, err = e.CreateKey(ctx, "azure", []byte(`{"name":"legacy","azure_resource_id":"/subscriptions/sub/resourceGroups/group/providers/Microsoft.CognitiveServices/accounts/account"}`))
	require.ErrorContains(t, err, "use azure_subscription_id")
}
