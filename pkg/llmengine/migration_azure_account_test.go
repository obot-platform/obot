package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func legacyEntraAccountSecrets() map[string]string {
	return map[string]string{
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT":        "https://endpoint.services.ai.azure.com/anthropic/v1",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID":       "client",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET":   "env.literal",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID":       "tenant",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_SUBSCRIPTION_ID": "legacy-subscription",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_RESOURCE_GROUP":  "legacy-group",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_RESOURCE_NAME":   "legacy-account",
	}
}

func TestLegacyAzureAccountMetadata(t *testing.T) {
	migrationDNS(t)
	for _, tc := range []struct {
		name    string
		source  string
		omit    []string
		invalid string
		want    bool
		wantErr bool
	}{
		{
			name:   "complete account",
			source: system.AzureEntraModelProvider,
			want:   true,
		},
		{
			name:   "older source without discovery",
			source: system.AzureEntraModelProvider,
			omit:   []string{"SUBSCRIPTION_ID", "RESOURCE_GROUP", "RESOURCE_NAME"},
		},
		{
			name:    "partial account",
			source:  system.AzureEntraModelProvider,
			omit:    []string{"RESOURCE_NAME"},
			wantErr: true,
		},
		{
			name:    "invalid account",
			source:  system.AzureEntraModelProvider,
			invalid: "other/account",
			wantErr: true,
		},
		{
			name:   "API key ignores discovery metadata",
			source: system.AzureModelProvider,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secrets := legacyEntraAccountSecrets()
			for _, field := range tc.omit {
				delete(secrets, "OBOT_AZURE_ENTRA_MODEL_PROVIDER_"+field)
			}
			if tc.invalid != "" {
				secrets["OBOT_AZURE_ENTRA_MODEL_PROVIDER_RESOURCE_NAME"] = tc.invalid
			}
			if tc.source == system.AzureModelProvider {
				secrets["OBOT_AZURE_MODEL_PROVIDER_API_KEY"] = "api-key"
			}
			p, err := legacyAggregate(t.Context(), tc.source, secrets)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if !tc.want {
				require.Empty(t, p.AzureAccounts)
				return
			}
			require.Equal(t, map[string]llmcoreclient.AzureAccount{"imported-" + tc.source: {
				SubscriptionID: "legacy-subscription",
				ResourceGroup:  "legacy-group",
				AccountName:    "legacy-account",
			}}, p.AzureAccounts)
			require.Equal(t, "env.literal", p.Keys[0].AzureKeyConfig.ClientSecret.GetValue())
		})
	}
}

func TestMigrationAzureAccountRecovery(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "standalone", true: "pooled"}[pooled], func(t *testing.T) {
			ctx := t.Context()
			g := migrationCredentials(t)
			putLegacy(t, g, system.AzureEntraModelProvider, legacyEntraAccountSecrets())
			if pooled {
				putLegacy(t, g, system.AzureModelProvider, azureSecrets("https://other.openai.azure.com"))
			}
			interrupted := false
			creates := 0
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok {
						creates++
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if config, ok := obj.(*v1.LLMGatewayConfig); ok && !interrupted {
						var settings map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(config.Spec.Payload), &settings))
						state, err := decodeMigration(settings)
						require.NoError(t, err)
						if state.Sources[system.AzureEntraModelProvider].Imported {
							interrupted = true
							return errors.New("interrupted Azure outcome")
						}
					}
					return c.Update(ctx, obj, opts...)
				},
			}).Build()
			store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			e := New()
			e.store = store
			_, err = e.ImportLegacyModelProviders(ctx, nil, g)
			require.ErrorContains(t, err, "interrupted Azure outcome")
			before, err := store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			require.Len(t, before.AzureAccounts, 1)
			require.NotContains(t, before.AzureAccounts, "imported-"+system.AzureModelProvider)

			// Reload through a fresh store and engine, after the aggregate is durable but its outcome is not.
			reloaded, err := llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			recovered := New()
			require.NoError(t, recovered.Start(ctx, Options{Store: reloaded, DataDir: t.TempDir()}))
			t.Cleanup(func() { require.NoError(t, recovered.Shutdown(context.Background())) })
			for range 2 {
				results, err := recovered.ImportLegacyModelProviders(ctx, nil, g)
				require.NoError(t, err)
				for _, result := range results {
					require.Equal(t, "imported", result.Status)
				}
			}
			after, err := reloaded.GetProvider(ctx, "azure")
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 1, creates)
			loaded, err := reloaded.Load(ctx)
			require.NoError(t, err)
			require.Equal(t, before.AzureAccounts, loaded.Config.AzureAccounts[schemas.Azure])
			require.True(t, recovered.core.Load().ProviderApplied(schemas.Azure, loaded.Config))

			calls := 0
			recovered.discovery.credential = func(key schemas.Key) (azcore.TokenCredential, error) {
				require.Equal(t, "imported-"+system.AzureEntraModelProvider, key.ID)
				return discoveryToken{t: t}, nil
			}
			recovered.discovery.client = &http.Client{Transport: discoveryTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, "management.azure.com", r.URL.Host)
				require.Equal(t, "/subscriptions/legacy-subscription/resourceGroups/legacy-group/providers/Microsoft.CognitiveServices/accounts/legacy-account/deployments", r.URL.Path)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"value":[{"name":"custom-deployment","properties":{"provisioningState":"Succeeded"}}]}`)), Header: http.Header{}}, nil
			})}
			require.NoError(t, recovered.RefreshAzureDeployments(ctx))
			require.Equal(t, 1, calls)
			_, err = recovered.Sync(ctx)
			require.NoError(t, err)
			loaded, err = reloaded.Load(ctx)
			require.NoError(t, err)
			require.True(t, recovered.core.Load().ProviderApplied(schemas.Azure, loaded.Config))
			require.Equal(t, []string{"custom-deployment"}, loaded.Config.AzureDiscoveries[schemas.Azure]["imported-"+system.AzureEntraModelProvider].Names)
			models, err := recovered.ListModels(ctx)
			require.NoError(t, err)
			require.Len(t, models.Models, 1)
			require.Equal(t, "azure/custom-deployment", models.Models[0].ID)

			// Completed imports preserve deliberate account edits, including disabling discovery.
			require.NoError(t, reloaded.MutateProvider(ctx, "azure", func(p *llmcoreclient.Provider) error {
				p.AzureAccounts = nil
				return nil
			}))
			_, err = recovered.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			after, err = reloaded.GetProvider(ctx, "azure")
			require.NoError(t, err)
			require.Empty(t, after.AzureAccounts)
		})
	}
}

func TestMigrationAzureAccountAfterAPIKeyCandidate(t *testing.T) {
	migrationDNS(t)
	e, ctx := startTestEngine(t)
	secrets := map[string]map[string]string{
		system.AzureModelProvider:      azureSecrets("https://other.openai.azure.com"),
		system.AzureEntraModelProvider: legacyEntraAccountSecrets(),
	}
	// Exercise the other assembly order: appending an Entra candidate must carry its account with its key.
	status, _, err := e.importLegacySources(ctx, []string{system.AzureModelProvider, system.AzureEntraModelProvider}, secrets, nil, false)
	require.NoError(t, err)
	require.Equal(t, "imported", status)
	p, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	candidate, err := legacyAggregate(ctx, system.AzureEntraModelProvider, secrets[system.AzureEntraModelProvider])
	require.NoError(t, err)
	require.Len(t, p.Keys, 2)
	require.Len(t, p.AzureAccounts, 1)
	require.Equal(t, candidate.AzureAccounts, p.AzureAccounts)
}
