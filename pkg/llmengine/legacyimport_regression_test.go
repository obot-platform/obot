package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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

	err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
	require.ErrorContains(t, err, "review credentials")
	require.ErrorContains(t, err, "azure_key_config.endpoint")
	require.ErrorContains(t, err, "169.254.169.254")
	require.ErrorContains(t, err, "which can't be used")
	require.NotContains(t, err.Error(), "legacy-secret")
	_, err = e.store.GetProvider(ctx, "azure")
	require.True(t, apierrors.IsNotFound(err))
	require.Zero(t, creates, "key validation must finish before the first durable write")
	require.Zero(t, deletes, "failed imports must never roll back by deleting a provider")
}

func TestLegacyImportUsesPrivateNetworkAllowance(t *testing.T) {
	for _, source := range []string{system.GenericResponsesModelProvider, system.AzureModelProvider} {
		for _, tc := range []struct {
			name         string
			endpoint     string
			allowPrivate bool
			wantErr      string
		}{
			{
				name:     "loopback denied",
				endpoint: "http://127.0.0.1:11434",
				wantErr:  "needs Allow private network addresses",
			},
			{
				name:         "loopback allowed",
				endpoint:     "http://127.0.0.1:11434",
				allowPrivate: true,
			},
			{
				name:     "private denied",
				endpoint: "https://10.1.2.3",
				wantErr:  "needs Allow private network addresses",
			},
			{
				name:         "private allowed",
				endpoint:     "https://10.1.2.3",
				allowPrivate: true,
			},
			{
				name:         "link-local metadata denied",
				endpoint:     "http://169.254.169.254",
				allowPrivate: true,
				wantErr:      "which can't be used",
			},
			{
				name:         "private metadata denied",
				endpoint:     "http://[fd00:ec2::254]",
				allowPrivate: true,
				wantErr:      "which can't be used",
			},
			{
				name:         "unspecified denied",
				endpoint:     "http://0.0.0.0",
				allowPrivate: true,
				wantErr:      "which can't be used",
			},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				e, ctx := startTestEngine(t)
				gateway := legacyImportGateway(t, map[string]map[string]string{
					source: {
						"OBOT_TEST_MODEL_PROVIDER_BASE_URL": tc.endpoint + "/v1/",
						"OBOT_TEST_MODEL_PROVIDER_ENDPOINT": tc.endpoint,
						"OBOT_TEST_MODEL_PROVIDER_API_KEY":  "legacy-secret",
					},
				})
				err := e.ImportLegacyModelProviders(ctx, nil, gateway, tc.allowPrivate)
				if tc.wantErr != "" {
					require.ErrorContains(t, err, tc.wantErr)
					_, err = e.store.GetProvider(ctx, legacyImportPlans[source].provider)
					require.True(t, apierrors.IsNotFound(err))
					return
				}
				require.NoError(t, err)
				provider, err := e.store.GetProvider(ctx, legacyImportPlans[source].provider)
				require.NoError(t, err)
				require.True(t, provider.Config.NetworkConfig.AllowPrivateNetwork)
				if source == system.GenericResponsesModelProvider {
					require.Equal(t, tc.endpoint, provider.Config.NetworkConfig.BaseURL)
					require.Equal(t, 300, provider.Config.NetworkConfig.DefaultRequestTimeoutInSeconds)
				} else {
					require.Equal(t, tc.endpoint, provider.Keys[0].AzureKeyConfig.Endpoint.GetValue())
				}
			})
		}
	}
}

func TestLegacyImportPreservesStorageErrorCauses(t *testing.T) {
	for _, operation := range []string{"read", "save"} {
		t.Run(operation, func(t *testing.T) {
			t.Setenv(system.OpenAIAPIKeyEnvVar, "")
			e, ctx := startTestEngine(t)
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.OpenAIModelProvider: {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "legacy-secret"},
			})
			cause := errors.New("storage unavailable")
			e.store = legacyImportInterceptStore(t, interceptor.Funcs{
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok && operation == "read" {
						return cause
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok && operation == "save" {
						return cause
					}
					return c.Create(ctx, obj, opts...)
				},
			})
			err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
			require.ErrorIs(t, err, cause)
			require.ErrorContains(t, err, operation)
			require.NotContains(t, err.Error(), "legacy-secret")
		})
	}
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

			err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
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
	err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
	require.NoError(t, err)
	require.Equal(t, 1, creates)
	require.Zero(t, updates)
	require.NoError(t, e.store.DeleteProvider(ctx, "bedrock"))
	err = e.ImportLegacyModelProviders(ctx, nil, gateway, false)
	require.NoError(t, err)
	require.Equal(t, 1, creates)
	_, err = e.store.GetProvider(ctx, "bedrock")
	require.True(t, apierrors.IsNotFound(err))
}

func TestLegacyImportRequiresExplicitBedrockIAMCredentialsAndRetries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing []string
	}{
		{
			name:    "missing access-key ID",
			missing: []string{"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID"},
		},
		{
			name:    "missing secret-access key",
			missing: []string{"OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"},
		},
		{
			name:    "both missing",
			missing: []string{"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID", "OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			iam := map[string]string{
				"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access-key",
				"OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret-key",
			}
			iam["OBOT_BEDROCK_MODEL_PROVIDER_REGION"] = "us-west-2"
			for _, field := range tc.missing {
				delete(iam, field)
			}
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.AmazonBedrockAPIKeyModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "api-key"},
				system.AmazonBedrockModelProvider:       iam,
			})
			require.ErrorContains(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false), "requires both access-key ID and secret-access key")
			_, err := e.store.GetProvider(ctx, "bedrock")
			require.True(t, apierrors.IsNotFound(err), "the valid API-key sibling must not be saved on its own")

			// Re-running after correction proves the failed family was not marked complete.
			iam["OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID"] = "access-key"
			iam["OBOT_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"] = "secret-key"
			require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{
				Context: system.AmazonBedrockModelProvider,
				Name:    system.AmazonBedrockModelProvider,
				Secrets: iam,
			}))
			require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
			provider, err := e.store.GetProvider(ctx, "bedrock")
			require.NoError(t, err)
			require.Len(t, provider.Keys, 2)
			for _, key := range provider.Keys {
				if key.Name == "imported-"+system.AmazonBedrockModelProvider {
					require.Equal(t, "access-key", key.BedrockKeyConfig.AccessKey.GetValue())
					require.Equal(t, "secret-key", key.BedrockKeyConfig.SecretKey.GetValue())
				}
			}
		})
	}
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

	err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
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
	require.Error(t, e.ImportLegacyModelProviders(interrupted, nil, gateway, false))
	first, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
	second, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.NoError(t, e.store.DeleteProvider(ctx, "openai"))
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
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
			err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
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
	err := e.ImportLegacyModelProviders(ctx, nil, gateway, false)
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
	require.Error(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
	_, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.NoError(t, e.store.DeleteProvider(ctx, "openai"))
	credentials[system.AzureModelProvider]["OBOT_AZURE_MODEL_PROVIDER_API_KEY"] = "corrected"
	require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.AzureModelProvider,
		Name:    system.AzureModelProvider,
		Secrets: credentials[system.AzureModelProvider],
	}))
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
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
	require.NoError(t, e.ImportLegacyModelProviders(ctx, storage, gateway, false))
	azure, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Equal(t, schemas.WhiteList{"my-deployment"}, azure.Keys[0].Models)
	var after v1.Model
	require.NoError(t, storage.Get(ctx, kclient.ObjectKeyFromObject(source), &after))
	require.Equal(t, source.Spec, after.Spec)
}

func TestLegacyAzureImportPreservesDeploymentDiscoveryMode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		source    string
		automatic bool
		want      []string
	}{
		{
			name:      "automatic Entra includes newly discovered deployments",
			source:    system.AzureEntraModelProvider,
			automatic: true,
			want:      []string{"azure/my-deployment", "azure/new-deployment"},
		},
		{
			name:   "manual Entra retains explicit deployments",
			source: system.AzureEntraModelProvider,
			want:   []string{"azure/my-deployment"},
		},
		{
			name:   "API key retains explicit deployments",
			source: system.AzureModelProvider,
			want:   []string{"azure/my-deployment"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			secrets := map[string]string{"OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://account.openai.azure.com"}
			if tc.source == system.AzureEntraModelProvider {
				secrets["OBOT_AZURE_MODEL_PROVIDER_CLIENT_ID"] = "client"
				secrets["OBOT_AZURE_MODEL_PROVIDER_CLIENT_SECRET"] = "secret"
				secrets["OBOT_AZURE_MODEL_PROVIDER_TENANT_ID"] = "tenant"
			} else {
				secrets["OBOT_AZURE_MODEL_PROVIDER_API_KEY"] = "api-key"
			}
			if tc.automatic {
				secrets["OBOT_AZURE_MODEL_PROVIDER_SUBSCRIPTION_ID"] = "subscription"
				secrets["OBOT_AZURE_MODEL_PROVIDER_RESOURCE_GROUP"] = "group"
				secrets["OBOT_AZURE_MODEL_PROVIDER_RESOURCE_NAME"] = "account"
			}
			gateway := legacyImportGateway(t, map[string]map[string]string{tc.source: secrets})
			source := migrationModel("m1-legacy-deployment", tc.source, "my-deployment")
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(source).Build()
			require.NoError(t, e.ImportLegacyModelProviders(ctx, storage, gateway, false))
			mockAzureDiscovery(t, e, func(*http.Request) (*http.Response, error) {
				return azureResponse(http.StatusOK, "my-deployment", "new-deployment"), nil
			})
			require.NoError(t, e.RefreshAzureDeployments(ctx))
			cfg, err := e.config(ctx)
			require.NoError(t, err)
			models, azure := e.azureDeploymentModels(cfg, schemas.Azure)
			require.True(t, azure)
			var ids []string
			for _, model := range models {
				ids = append(ids, model.ID)
			}
			require.Equal(t, tc.want, ids)
		})
	}
}

func TestLegacyImportPreservesExistingDestinationBeforeValidatingSource(t *testing.T) {
	for _, name := range []string{"bedrock", "bedrock_mantle"} {
		t.Run(name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			t.Setenv("HTTPS_PROXY", "http://environment-proxy.invalid:3128")
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.AmazonBedrockModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID": "incomplete-credentials"},
			})
			_, err := e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": name, "network_config": map[string]any{"max_retries": 7}, "proxy_config": map[string]any{"type": "http", "url": "http://admin-proxy.invalid:8080"}}))
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, name)
			require.NoError(t, err)
			require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, true))
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

func TestLegacyImportAfterStartupAppliesProxyBeforePublication(t *testing.T) {
	for _, tc := range []struct {
		name      string
		noProxy   string
		wantProxy bool
	}{
		{
			name:      "environment proxy",
			wantProxy: true,
		},
		{
			name:    "NO_PROXY bypass",
			noProxy: "*",
		},
		{
			name:      "NO_PROXY custom endpoint",
			noProxy:   "8.8.8.8",
			wantProxy: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(system.OpenAIAPIKeyEnvVar, "")
			t.Setenv(system.AnthropicAPIKeyEnvVar, "")
			e, ctx := startTestEngine(t)
			for _, name := range []string{"HTTP_PROXY", "http_proxy", "https_proxy", "no_proxy"} {
				t.Setenv(name, "")
			}
			t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")
			t.Setenv("NO_PROXY", tc.noProxy)
			published := 0
			e.store = legacyImportInterceptStore(t, interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if resource, ok := obj.(*v1.LLMGatewayProvider); ok {
						var provider llmcoreclient.Provider
						require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &provider))
						// A watcher can synchronize this aggregate as soon as Create publishes it.
						wantProxy := tc.wantProxy
						if resource.Spec.ID == "generic-responses" && tc.noProxy == "8.8.8.8" {
							wantProxy = false
						}
						if wantProxy {
							require.NotNil(t, provider.Config.ProxyConfig, resource.Spec.ID)
							require.Equal(t, "http://proxy.invalid:3128", provider.Config.ProxyConfig.URL.GetValue())
						} else {
							require.True(t, provider.Config.ProxyConfig == nil || provider.Config.ProxyConfig.Type == schemas.NoProxy)
						}
						published++
					}
					return c.Create(ctx, obj, opts...)
				},
			})
			mp := ModelProviderResource()
			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
			changed, _ := e.startupChanges(ctx, storage, mp)()
			require.False(t, changed, "startup ran before any legacy providers existed")
			gateway := legacyImportGateway(t, map[string]map[string]string{
				system.OpenAIModelProvider:              {"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "test"},
				system.AnthropicModelProvider:           {"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": "test"},
				system.AzureModelProvider:               {"OBOT_AZURE_MODEL_PROVIDER_API_KEY": "test", "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://8.8.8.8"},
				system.AmazonBedrockAPIKeyModelProvider: {"OBOT_BEDROCK_MODEL_PROVIDER_API_KEY": "test"},
				system.GenericResponsesModelProvider:    {"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://8.8.8.8/v1"},
			})
			require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
			require.Equal(t, 5, published)
		})
	}
}

func TestLegacyImportProxyBeforeFirstDiscoveryAndInference(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	upstream := mockOpenAI(t)
	t.Cleanup(upstream.Close)
	// Bifrost tunnels HTTP through CONNECT too. Always tunnel to the local mock,
	// regardless of the requested address, so no test traffic can reach the cloud.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "8.8.8.8:80" {
			http.Error(w, "unexpected proxy target", http.StatusBadRequest)
			return
		}
		conn, err := net.Dial("tcp", upstream.Listener.Addr().String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer conn.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			t.Error(err)
			return
		}
		if err := buffered.Flush(); err != nil {
			t.Error(err)
			return
		}
		go func() { _, _ = io.Copy(conn, buffered) }()
		_, _ = io.Copy(client, conn)
	}))
	t.Cleanup(proxy.Close)
	for _, name := range []string{"HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(name, "")
	}
	t.Setenv("HTTP_PROXY", proxy.URL)
	mp := ModelProviderResource()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
	changed, _ := e.startupChanges(ctx, storage, mp)()
	require.False(t, changed)
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.GenericResponsesModelProvider: {"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "http://8.8.8.8/v1"},
	})
	require.NoError(t, e.ImportLegacyModelProviders(ctx, nil, gateway, false))
	provider, err := e.store.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	require.NotNil(t, provider.Config.ProxyConfig, "stop before network use if the import lacks its proxy")
	require.Equal(t, proxy.URL, provider.Config.ProxyConfig.URL.GetValue())
	// Reconcile immediately, as a provider watch does, before the post-import model-sync trigger.
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	models, err := e.ListModels(ctx)
	require.NoError(t, err)
	var ids []string
	for _, model := range models.Models {
		ids = append(ids, model.ID)
	}
	require.Contains(t, ids, "generic-responses/gpt-4o-mini")
	code, response := do(t, e, http.MethodPost, "/v1/responses", `{"model":"generic-responses/gpt-4o-mini","input":"hi"}`)
	require.Equal(t, http.StatusOK, code, response)
	require.Contains(t, response, "Hello from responses")
}
