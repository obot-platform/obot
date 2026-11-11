package llmengine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func legacyImportGateway(t *testing.T, credentials map[string]map[string]string) *gatewayclient.Client {
	t.Helper()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), db, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gateway.Close() })
	for name, secrets := range credentials {
		require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: name,
			Name:    name,
			Secrets: secrets,
		}))
	}
	return gateway
}

func TestLegacyImportValidatesKeyBeforeCreatingProvider(t *testing.T) {
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.AzureModelProvider: {
			"OBOT_AZURE_MODEL_PROVIDER_API_KEY":  "legacy-secret",
			"OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "http://169.254.169.254",
		},
	})
	creates, deletes := 0, 0
	e.store = legacyImportInterceptStore(t, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok {
				creates++
			}
			return c.Create(ctx, obj, opts...)
		},
		Delete: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.DeleteOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok {
				deletes++
			}
			return c.Delete(ctx, obj, opts...)
		},
	})

	err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.ErrorContains(t, err, "review credentials")
	_, err = e.store.GetProvider(ctx, "azure")
	require.True(t, apierrors.IsNotFound(err))
	require.Zero(t, creates, "key validation must finish before the first durable write")
	require.Zero(t, deletes, "failed imports must never roll back by deleting a provider")
}

func legacyImportInterceptStore(t *testing.T, funcs interceptor.Funcs) *llmcoreclient.Client {
	t.Helper()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	store, err := llmcoreclient.New(interceptor.NewClient(storage, funcs), plaintextConfigCodec{})
	require.NoError(t, err)
	return store
}

func TestLegacyImportCreateConflictPreservesConcurrentProvider(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent creation", true: "same-name recreation"}[recreate], func(t *testing.T) {
			t.Setenv(system.OpenAIAPIKeyEnvVar, "")
			e, ctx := startTestEngine(t)
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.OpenAIModelProvider: {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "env.literal-import-secret"},
			})
			injected := false
			e.store = legacyImportInterceptStore(t, interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if p, ok := obj.(*v1.LLMGatewayProvider); ok && !injected {
						injected = true
						var aggregate llmcoreclient.Provider
						require.NoError(t, json.Unmarshal([]byte(p.Spec.Payload), &aggregate))
						require.Len(t, aggregate.Keys, 1, "the first write must contain the validated key")
						require.Equal(t, "env.literal-import-secret", aggregate.Keys[0].Value.GetValue())
						require.Equal(t, "plain_text", string(aggregate.Keys[0].Value.Type()))
						require.NotEmpty(t, aggregate.Keys[0].ID)

						_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
						require.NoError(t, err)
						if recreate {
							require.NoError(t, e.DeleteProvider(ctx, "openai"))
							_, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai","network_config":{"max_retries":7}}`))
							require.NoError(t, err)
						}
						_, err = e.CreateKey(ctx, "openai", json.RawMessage(`{"name":"independent","value":"concurrent-secret"}`))
						require.NoError(t, err)
						err = c.Create(ctx, obj, opts...)
						require.True(t, apierrors.IsAlreadyExists(err))
						require.EqualValues(t, http.StatusConflict, storageError(err).(*apierrors.StatusError).ErrStatus.Code)
						return err
					}
					return c.Create(ctx, obj, opts...)
				},
			})

			err := e.ImportLegacyModelProviders(ctx, nil, gateway)
			require.NoError(t, err)
			require.True(t, injected)
			provider, err := e.store.GetProvider(ctx, "openai")
			require.NoError(t, err)
			require.Len(t, provider.Keys, 1)
			require.Equal(t, "concurrent-secret", provider.Keys[0].Value.GetValue())
			if recreate {
				require.Equal(t, 7, provider.Config.NetworkConfig.MaxRetries)
			}
		})
	}
}

func TestLegacyImportCreatesFamilyInOneWriteAndDoesNotReimportDeletion(t *testing.T) {
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.AmazonBedrockAPIKeyModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "vault.literal-secret"},
		system.AmazonBedrockModelProvider:       {"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID": "access-key", "OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret-key"},
	})
	creates, updates := 0, 0
	e.store = legacyImportInterceptStore(t, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			if p, ok := obj.(*v1.LLMGatewayProvider); ok {
				creates++
				var aggregate llmcoreclient.Provider
				require.NoError(t, json.Unmarshal([]byte(p.Spec.Payload), &aggregate))
				require.Len(t, aggregate.Keys, 2)
			}
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok {
				updates++
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.NoError(t, err)
	require.Equal(t, 1, creates)
	require.Zero(t, updates)
	require.NoError(t, e.store.DeleteProvider(ctx, "bedrock"))
	err = e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.NoError(t, err)
	require.Equal(t, 1, creates)
	_, err = e.store.GetProvider(ctx, "bedrock")
	require.True(t, apierrors.IsNotFound(err))
}

func TestLegacyImportGroupedProviders(t *testing.T) {
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.AmazonBedrockAPIKeyModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "vault.literal-secret"},
		system.AmazonBedrockModelProvider: {
			"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access-key",
			"OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret-key",
		},
		system.AzureModelProvider: {
			"OBOT_AZURE_MODEL_PROVIDER_API_KEY":  "env.literal-secret",
			"OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://8.8.8.8",
		},
		system.AzureEntraModelProvider: {
			"OBOT_AZURE_MODEL_PROVIDER_ENDPOINT":      "https://8.8.4.4",
			"OBOT_AZURE_MODEL_PROVIDER_CLIENT_ID":     "client-id",
			"OBOT_AZURE_MODEL_PROVIDER_CLIENT_SECRET": "vault.literal-client-secret",
			"OBOT_AZURE_MODEL_PROVIDER_TENANT_ID":     "tenant-id",
		},
	})

	err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.NoError(t, err)
	for _, name := range []string{"bedrock", "azure"} {
		provider, err := e.store.GetProvider(ctx, name)
		require.NoError(t, err)
		require.Len(t, provider.Keys, 2)
		require.Equal(t, defaultMaxRetries, provider.Config.NetworkConfig.MaxRetries)
		require.Equal(t, defaultRequestTimeout, provider.Config.NetworkConfig.DefaultRequestTimeoutInSeconds)
		for _, key := range provider.Keys {
			require.NotEmpty(t, key.ID)
			require.Equal(t, float64(1), key.Weight)
			require.Equal(t, schemas.WhiteList{"*"}, key.Models)
		}
		view, err := e.GetProvider(ctx, name)
		require.NoError(t, err)
		for _, key := range view.Keys {
			if key.Value.GetValue() != "" {
				require.True(t, key.Value.IsRedacted())
			}
		}
	}
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	providers, err := e.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, providers, schemas.Bedrock, "normal sync must load the imported configuration")
	require.Contains(t, providers, schemas.Azure)
}

func TestLegacyImportRetriesAfterCompletionWriteFails(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{system.OpenAIModelProvider: {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "secret"}})
	interrupted, cancel := context.WithCancel(ctx)
	defer cancel()
	e.store = legacyImportInterceptStore(t, interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			err := c.Create(ctx, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && err == nil {
				cancel()
			}
			return err
		},
	})
	require.Error(t, e.ImportLegacyModelProviders(interrupted, nil, gateway))
	first, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway))
	second, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, e.store.DeleteProvider(ctx, "openai"))
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway))
	_, err = e.store.GetProvider(ctx, "openai")
	require.True(t, apierrors.IsNotFound(err))
}

func TestAzureImportRetainsAccountMetadataAndRejectsInvalidSibling(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(fmt.Sprint(invalid), func(t *testing.T) {
			e, ctx := startTestEngine(t)
			credentials := map[string]map[string]string{
				system.AzureModelProvider:      {"OBOT_AZURE_MODEL_PROVIDER_API_KEY": "key", "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://8.8.8.8"},
				system.AzureEntraModelProvider: {"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT": "https://account.openai.azure.com", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID": "client", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID": "tenant", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_SUBSCRIPTION_ID": "subscription", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_RESOURCE_GROUP": "group", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_RESOURCE_NAME": "account"},
			}
			if invalid {
				credentials[system.AzureEntraModelProvider]["OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET"] = ""
			}
			gateway := legacyImportGateway(t, credentials)
			err := e.ImportLegacyModelProviders(ctx, nil, gateway)
			if invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			provider, err := e.store.GetProvider(ctx, "azure")
			if invalid {
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err)
			require.Len(t, provider.Keys, 2)
			require.Len(t, provider.AzureAccounts, 1)
			for _, account := range provider.AzureAccounts {
				require.Equal(t, llmcoreclient.AzureAccount{SubscriptionID: "subscription", ResourceGroup: "group", AccountName: "account"}, account)
			}
		})
	}
}

func TestLegacyKeylessGenericResponses(t *testing.T) {
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{system.GenericResponsesModelProvider: {"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://8.8.8.8/custom/v1/"}})
	err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.NoError(t, err)
	provider, err := e.store.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	require.True(t, provider.Config.CustomProviderConfig.IsKeyLess)
	require.Equal(t, "https://8.8.8.8/custom", provider.Config.NetworkConfig.BaseURL)
	require.Empty(t, provider.Keys[0].Value.GetValue())
}

func TestLegacyImportRetriesFailedFamilyWithoutReplayingCompletedFamily(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	credentials := map[string]map[string]string{
		system.OpenAIModelProvider: {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "secret"},
		system.AzureModelProvider:  {"OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://8.8.8.8"},
	}
	gateway := legacyImportGateway(t, credentials)
	require.Error(t, e.ImportLegacyModelProviders(ctx, nil, gateway))
	_, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.NoError(t, e.store.DeleteProvider(ctx, "openai"))
	credentials[system.AzureModelProvider]["OBOT_AZURE_MODEL_PROVIDER_API_KEY"] = "corrected"
	require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.AzureModelProvider,
		Name:    system.AzureModelProvider,
		Secrets: credentials[system.AzureModelProvider],
	}))
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway))
	azure, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Equal(t, "corrected", azure.Keys[0].Value.GetValue())
	_, err = e.store.GetProvider(ctx, "openai")
	require.True(t, apierrors.IsNotFound(err))
}

func TestLegacyImportReadsAzureDeploymentsWithoutChangingSources(t *testing.T) {
	e, ctx := startTestEngine(t)
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.AzureModelProvider: {"OBOT_AZURE_MODEL_PROVIDER_API_KEY": "secret", "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://8.8.8.8"},
	})
	source := &v1.Model{
		Namespace: system.DefaultNamespace,
		Name:      "legacy-deployment",
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: system.AzureModelProvider,
			TargetModel:   "my-deployment",
			Active:        true,
		}},
	}
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(source).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(context.Context, kclient.WithWatch, kclient.Object, ...kclient.UpdateOption) error {
			t.Fatal("provider import must not update source models or references")
			return nil
		},
		Delete: func(context.Context, kclient.WithWatch, kclient.Object, ...kclient.DeleteOption) error {
			t.Fatal("provider import must not delete source records")
			return nil
		},
	}).Build()
	require.NoError(t, e.ImportLegacyModelProviders(ctx, storage, gateway))
	azure, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Equal(t, schemas.WhiteList{"my-deployment"}, azure.Keys[0].Models)
	var after v1.Model
	require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(source), &after))
	require.Equal(t, source.Spec, after.Spec)
}

func TestLegacyImportPreservesExistingDestinationBeforeValidatingSource(t *testing.T) {
	for _, name := range []string{"bedrock", "bedrock_mantle"} {
		t.Run(name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.AmazonBedrockModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID": "incomplete-credentials"},
			})
			_, err := e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": name, "network_config": map[string]any{"max_retries": 7}}))
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, name)
			require.NoError(t, err)
			require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway))
			after, err := e.store.GetProvider(ctx, name)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if name == "bedrock_mantle" {
				_, err = e.store.GetProvider(ctx, "bedrock")
				require.True(t, apierrors.IsNotFound(err))
			}
		})
	}
}
