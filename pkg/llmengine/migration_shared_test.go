package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestMigrationSharedPool(t *testing.T) {
	for _, tc := range []struct {
		family  string
		secrets map[string]map[string]string
	}{
		{
			family: "azure",
			secrets: map[string]map[string]string{
				system.AzureModelProvider: azureSecrets("https://one.openai.azure.com"),
				system.AzureEntraModelProvider: {
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT":      "https://two.services.ai.azure.com",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID":     "client",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID":     "tenant",
				},
			},
		},
		{
			family: "bedrock_mantle",
			secrets: map[string]map[string]string{
				system.AmazonBedrockModelProvider: {
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access",
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret",
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION":            "eu-west-1",
				},
				system.AmazonBedrockAPIKeyModelProvider: {
					"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY": "vault.literal",
					"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_REGION":  "us-east-1",
				},
			},
		},
	} {
		t.Run(tc.family, func(t *testing.T) {
			ctx := t.Context()
			creates := 0
			failOutcome := false
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if resource, ok := obj.(*v1.LLMGatewayProvider); ok {
						var p llmcoreclient.Provider
						require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &p))
						require.Len(t, p.Keys, 2, "both keys must be present in the first durable version")
						require.Equal(t, tc.family, p.Migration.Source)
						require.Equal(t, "both", p.Migration.Decision)
						require.Equal(t, schemas.EnvProxy, p.Config.ProxyConfig.Type)
						creates++
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if resource, ok := obj.(*v1.LLMGatewayConfig); ok && failOutcome {
						var settings map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &settings))
						state, err := decodeMigration(settings)
						require.NoError(t, err)
						for _, source := range state.Sources {
							if source.Imported {
								failOutcome = false
								return errors.New("interrupted outcome write")
							}
						}
					}
					return c.Update(ctx, obj, opts...)
				},
			}).Build()
			store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			e := New()
			require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
			t.Cleanup(e.Shutdown)
			t.Setenv("HTTPS_PROXY", "http://egress.invalid:3128")
			t.Setenv("NO_PROXY", "one.openai.azure.com,bedrock-mantle.eu-west-1.api.aws")
			g := migrationCredentials(t)
			for name, secrets := range tc.secrets {
				putLegacy(t, g, name, secrets)
			}
			_, err = e.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			require.Zero(t, creates, "shared import must be explicitly selected")
			state, err := e.MigrationStatus(ctx)
			require.NoError(t, err)
			require.NoError(t, e.ResolveMigration(ctx, tc.family, "both", state.Version))
			require.ErrorContains(t, e.ResolveMigration(ctx, tc.family, "both", state.Version), "decision changed")
			failOutcome = true
			_, err = e.ImportLegacyModelProviders(ctx, nil, g)
			require.ErrorContains(t, err, "interrupted outcome write")
			require.Equal(t, 1, creates)

			// A fresh reconciler recovers the saved decision and complete aggregate.
			recovered := New()
			recovered.store = store
			results, err := recovered.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			require.Len(t, results, 2)
			for _, result := range results {
				require.Equal(t, "imported", result.Status)
				provider, err := recovered.LegacyGatewayProvider(ctx, result.ModelProvider)
				require.NoError(t, err)
				require.Equal(t, tc.family, provider, "both legacy routes resolve to the shared pool")
			}
			p, err := store.GetProvider(ctx, tc.family)
			require.NoError(t, err)
			require.Len(t, p.Keys, 2)
			for _, key := range p.Keys {
				require.Equal(t, schemas.WhiteList{"*"}, key.Models)
				switch key.ID {
				case "imported-" + system.AzureModelProvider:
					require.Equal(t, "https://one.openai.azure.com", key.AzureKeyConfig.Endpoint.GetValue())
					require.Equal(t, "vault.literal", key.Value.GetValue())
				case "imported-" + system.AzureEntraModelProvider:
					require.Equal(t, "https://two.services.ai.azure.com", key.AzureKeyConfig.Endpoint.GetValue())
					require.Equal(t, "secret", key.AzureKeyConfig.ClientSecret.GetValue())
				case "imported-" + system.AmazonBedrockModelProvider:
					require.Equal(t, "eu-west-1", key.BedrockMantleKeyConfig.Region.GetValue())
					require.Equal(t, "access", key.BedrockMantleKeyConfig.AccessKey.GetValue())
				case "imported-" + system.AmazonBedrockAPIKeyModelProvider:
					require.Equal(t, "us-east-1", key.BedrockMantleKeyConfig.Region.GetValue())
					require.Equal(t, "vault.literal", key.Value.GetValue())
				default:
					t.Fatalf("unexpected imported key %q", key.ID)
				}
			}
			_, err = e.Sync(ctx)
			require.NoError(t, err)
			require.NotNil(t, e.Client().GetProviderByKey(schemas.ModelProvider(tc.family)))
			require.False(t, e.ApplyEgressProxy(ctx))
			require.NoError(t, store.MutateProvider(ctx, tc.family, func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = 123
				p.Config.ProxyConfig = &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://admin.invalid:8080")}
				return nil
			}))
			before, err := store.GetProvider(ctx, tc.family)
			require.NoError(t, err)
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for range 2 {
				wg.Go(func() { _, err := e.ImportLegacyModelProviders(ctx, nil, g); errs <- err })
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			after, err := store.GetProvider(ctx, tc.family)
			require.NoError(t, err)
			require.Equal(t, before, after, "retries preserve keys and administrator edits")
			require.Equal(t, 1, creates)
			require.NoError(t, e.mutateMigration(ctx, func(s *MigrationStatus) error {
				// Simulate a crash after only one source's successful outcome was recorded.
				for name, source := range s.Sources {
					source.Imported = false
					s.Sources[name] = source
					break
				}
				return nil
			}))
			require.NoError(t, store.DeleteProvider(ctx, tc.family))
			results, err = e.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			for _, result := range results {
				require.Equal(t, "blocked", result.Status)
			}
			require.Equal(t, 1, creates, "a deleted completed pool must not be recreated")
		})
	}
}

func TestMigrationSharedPoolValidatesAllKeysBeforeWriting(t *testing.T) {
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.AzureModelProvider, azureSecrets("http://169.254.169.254"))
	putLegacy(t, g, system.AzureEntraModelProvider, map[string]string{
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT":      "https://two.services.ai.azure.com",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID":     "client",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID":     "tenant",
	})
	_, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	state, err := e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.NoError(t, e.ResolveMigration(ctx, "azure", "both", state.Version))
	_, err = e.ImportLegacyModelProviders(ctx, nil, g)
	require.ErrorContains(t, err, "link-local")
	_, err = e.store.GetProvider(ctx, "azure")
	require.True(t, apierrors.IsNotFound(err), "a valid first key must not be committed before the second is validated")
}

func TestMigrationSharedPoolPreservesExistingProvider(t *testing.T) {
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.AmazonBedrockModelProvider, map[string]string{
		"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access",
		"OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret",
	})
	_, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	state, err := e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.ErrorContains(t, e.ResolveMigration(ctx, "bedrock_mantle", "both", state.Version), "no longer awaiting a choice")
	before, err := e.store.GetProvider(ctx, "bedrock_mantle")
	require.NoError(t, err)
	putLegacy(t, g, system.AmazonBedrockAPIKeyModelProvider, map[string]string{"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "api-key"})
	_, err = e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	state, err = e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.NoError(t, e.ResolveMigration(ctx, "bedrock_mantle", "both", state.Version))
	results, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	for _, result := range results {
		require.Equal(t, "blocked", result.Status)
	}
	after, err := e.store.GetProvider(ctx, "bedrock_mantle")
	require.NoError(t, err)
	require.Equal(t, before, after, "a shared decision cannot extend an existing single import")
}

func TestSharedAzurePoolEgressHonorsNoProxy(t *testing.T) {
	e, ctx := startTestEngine(t)
	var requests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(http.StatusOK)
			return
		}
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat-1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"proxied"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", "")
	t.Setenv("https_proxy", "")
	t.Setenv("NO_PROXY", "10.1.2.3")
	p := llmcoreclient.Provider{}
	p.Config.CheckAndSetDefaults()
	p.Config.NetworkConfig.MaxRetries = 0
	for id, endpoint := range map[string]string{"proxied": "http://pool-proxied.invalid", "direct": "http://10.1.2.3"} {
		p.Keys = append(p.Keys, schemas.Key{
			ID:             id,
			Name:           id,
			Value:          *schemas.NewSecretVar("key"),
			Models:         schemas.WhiteList{"*"},
			Weight:         1,
			AzureKeyConfig: &schemas.AzureKeyConfig{Endpoint: *schemas.NewSecretVar(endpoint)},
		})
	}
	require.NoError(t, e.store.CreateProvider(ctx, "azure", p))
	require.True(t, e.ApplyEgressProxy(ctx))
	_, err := e.Sync(ctx)
	require.NoError(t, err)
	for _, id := range []string{"proxied", "direct"} {
		bctx, cancel := schemas.NewBifrostContextWithCancel(ctx)
		bctx.SetValue(schemas.BifrostContextKeyAPIKeyID, id)
		response, requestErr := e.Client().ChatCompletionRequest(bctx, &schemas.BifrostChatRequest{
			Provider: schemas.Azure,
			Model:    "gpt-4o",
			Input:    []schemas.ChatMessage{{Role: schemas.ChatMessageRoleUser, Content: &schemas.ChatMessageContent{ContentStr: new("hello")}}},
		})
		cancel()
		if id == "proxied" {
			require.Nil(t, requestErr)
			require.NotNil(t, response)
		} else {
			// The excluded destination goes direct and is rejected by the private-network policy.
			require.NotNil(t, requestErr)
		}
		require.Equal(t, int32(1), requests.Load(), "NO_PROXY must bypass the proxy for the second key")
	}
}
