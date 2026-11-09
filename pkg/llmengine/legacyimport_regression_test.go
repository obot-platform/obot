package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	types "github.com/obot-platform/obot/apiclient/types"
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

	results, err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.ErrorContains(t, err, "link-local")
	require.Len(t, results, 1)
	require.Equal(t, "error", results[0].Status)
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

			results, err := e.ImportLegacyModelProviders(ctx, nil, gateway)
			require.NoError(t, err)
			require.True(t, injected)
			require.Len(t, results, 1)
			require.Equal(t, "blocked", results[0].Status)
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

func TestLegacyImportCloudProvidersPoolKeys(t *testing.T) {
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

	results, err := e.ImportLegacyModelProviders(ctx, nil, gateway)
	require.NoError(t, err)
	require.Len(t, results, 4)
	for _, result := range results {
		require.Equal(t, "imported", result.Status, result.Message)
	}
	for _, name := range []string{"bedrock_mantle", "azure"} {
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
	require.Contains(t, providers, schemas.BedrockMantle, "reconciliation activates the imported pool")
	require.Contains(t, providers, schemas.Azure)
}

func TestLegacyAWSImportPreservesExistingBedrock(t *testing.T) {
	ctx := t.Context()
	source := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-legacy-aws", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.AmazonBedrockAPIKeyModelProvider, TargetModel: "claude-sonnet-4-5", Active: true}}}
	provider := &v1.ModelProvider{Namespace: system.DefaultNamespace, Name: system.AmazonBedrockAPIKeyModelProvider}
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(source, provider).WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(obj kclient.Object) []string { return []string{obj.(*v1.Model).Spec.Manifest.ModelProvider} }).Build()
	store, err := llmcoreclient.New(c, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	gateway := legacyImportGateway(t, map[string]map[string]string{
		system.AmazonBedrockAPIKeyModelProvider: {"OBOT_AWS_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY": "legacy-key"},
	})
	var current llmcoreclient.Provider
	require.NoError(t, json.Unmarshal([]byte(`{"config":{"network_config":{"default_request_timeout_in_seconds":71}},"keys":[{"id":"intentional","name":"current","value":"current-key","models":["anthropic.claude-test"],"weight":1,"bedrock_key_config":{"region":"us-west-2"}}]}`), &current))
	require.NoError(t, e.store.CreateProvider(ctx, "bedrock", current))
	before, err := e.store.GetProvider(ctx, "bedrock")
	require.NoError(t, err)
	for range 2 {
		results, err := e.ImportLegacyModelProviders(ctx, c, gateway)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "blocked", results[0].Status)
		require.Contains(t, results[0].Message, "existing")
		after, err := e.store.GetProvider(ctx, "bedrock")
		require.NoError(t, err)
		require.Equal(t, before, after)
		require.NoError(t, e.MigrateLegacyModels(ctx, c))
		var retained v1.Model
		require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(source), &retained))
		require.Equal(t, source.Spec, retained.Spec)
		var retainedProvider v1.ModelProvider
		require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(provider), &retainedProvider))
		_, err = e.store.GetProvider(ctx, "bedrock_mantle")
		require.True(t, apierrors.IsNotFound(err))
		status, err := e.MigrationStatus(ctx)
		require.NoError(t, err)
		require.False(t, status.Sources[system.AmazonBedrockAPIKeyModelProvider].Imported)
		sources, err := e.migratedSources(ctx)
		require.NoError(t, err)
		require.NotContains(t, sources, system.AmazonBedrockAPIKeyModelProvider, "unverified current keys cannot authorize cleanup")
		cred, err := gateway.RevealCredential(ctx, []string{system.AmazonBedrockAPIKeyModelProvider}, system.AmazonBedrockAPIKeyModelProvider)
		require.NoError(t, err)
		require.Equal(t, "legacy-key", cred.Secrets["OBOT_AWS_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY"])
	}

	t.Run("unreadable current configuration is an error", func(t *testing.T) {
		var providers v1.LLMGatewayProviderList
		require.NoError(t, c.List(ctx, &providers))
		require.Len(t, providers.Items, 1)
		providers.Items[0].Spec.Payload = "invalid JSON"
		require.NoError(t, c.Update(ctx, &providers.Items[0]))
		results, err := e.ImportLegacyModelProviders(ctx, c, gateway)
		require.Error(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "error", results[0].Status)
		_, err = e.store.GetProvider(ctx, "bedrock_mantle")
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestLegacyAWSImportKeepsExistingMantle(t *testing.T) {
	e, ctx := startTestEngine(t)
	g := legacyImportGateway(t, map[string]map[string]string{system.AmazonBedrockAPIKeyModelProvider: {"OBOT_AWS_MODEL_PROVIDER_API_KEY": "legacy-key"}})
	results, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "imported", results[0].Status)
	mantle, err := e.store.GetProvider(ctx, "bedrock_mantle")
	require.NoError(t, err)
	require.NoError(t, e.store.CreateProvider(ctx, "bedrock", llmcoreclient.Provider{}))
	for range 2 {
		results, err = e.ImportLegacyModelProviders(ctx, nil, g)
		require.NoError(t, err)
		require.Len(t, results, 1)
		require.Equal(t, "imported", results[0].Status)
		after, err := e.store.GetProvider(ctx, "bedrock_mantle")
		require.NoError(t, err)
		require.Equal(t, mantle, after)
		normal, err := e.store.GetProvider(ctx, "bedrock")
		require.NoError(t, err)
		require.Nil(t, normal.Migration)
		require.Empty(t, normal.Keys)
	}
}
