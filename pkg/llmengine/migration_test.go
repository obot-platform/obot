package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type failingMigrationCodec struct {
	plaintextConfigCodec
	fail bool
}

type legacyCredentialTransformer struct {
	fail bool
}

func (c *failingMigrationCodec) SealGatewayConfig(ctx context.Context, id string, b []byte) ([]byte, error) {
	if c.fail && strings.HasPrefix(id, "provider/") {
		return nil, errors.New("injected codec failure")
	}
	return c.plaintextConfigCodec.SealGatewayConfig(ctx, id, b)
}

func migrationCredentials(t *testing.T) *gatewayclient.Client {
	t.Helper()
	migrationDNS(t)
	storage, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(storage.DB.DB, storage.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func migrationDNS(t *testing.T) {
	t.Helper()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: testKeyDNS}
	t.Cleanup(func() { net.DefaultResolver = previous })
}
func putLegacy(t *testing.T, c *gatewayclient.Client, name string, secrets map[string]string) {
	t.Helper()
	require.NoError(t, c.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: name, Name: name, Secrets: secrets}))
}
func azureSecrets(endpoint string) map[string]string {
	return map[string]string{"OBOT_AZURE_MODEL_PROVIDER_API_KEY": "vault.literal", "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": endpoint}
}

func TestLegacyAggregateAllProviders(t *testing.T) {
	migrationDNS(t)
	for _, tc := range []struct {
		name     string
		secrets  map[string]string
		provider string
	}{
		{
			name:     system.OpenAIModelProvider,
			secrets:  map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "env.literal"},
			provider: "openai",
		},
		{
			name:     system.AnthropicModelProvider,
			secrets:  map[string]string{"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": "vault.literal"},
			provider: "anthropic",
		},
		{
			name:     system.GenericResponsesModelProvider,
			secrets:  map[string]string{"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY": "key", "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://example.com/prefix/v1/"},
			provider: "generic-responses",
		},
		{
			name:     system.AmazonBedrockModelProvider,
			secrets:  map[string]string{"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID": "access", "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret", "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN": "session", "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION": "eu-west-1"},
			provider: "bedrock_mantle",
		},
		{
			name:     system.AmazonBedrockAPIKeyModelProvider,
			secrets:  map[string]string{"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY": "key"},
			provider: "bedrock_mantle",
		},
		{
			name:     system.AzureModelProvider,
			secrets:  azureSecrets("https://example.openai.azure.com/openai/v1/"),
			provider: "azure",
		},
		{
			name:     system.AzureEntraModelProvider,
			secrets:  map[string]string{"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT": "https://example.services.ai.azure.com/anthropic/v1", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID": "client", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "env.literal", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID": "tenant"},
			provider: "azure",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := legacyAggregate(t.Context(), tc.name, tc.secrets)
			require.NoError(t, err)
			require.Len(t, p.Keys, 1)
			require.Equal(t, tc.name, p.Migration.Source)
			require.Equal(t, tc.provider, legacyImportPlans[tc.name].provider)
			require.Equal(t, schemas.WhiteList{"*"}, p.Keys[0].Models)
			require.Equal(t, "imported-"+tc.name, p.Keys[0].ID)
			if tc.provider == "azure" {
				require.Equal(t, "https://"+map[bool]string{true: "example.services.ai.azure.com", false: "example.openai.azure.com"}[tc.name == system.AzureEntraModelProvider], p.Keys[0].AzureKeyConfig.Endpoint.GetValue())
			}
			if tc.name == system.AzureEntraModelProvider {
				require.Equal(t, []string{"https://ai.azure.com/.default"}, p.Keys[0].AzureKeyConfig.Scopes)
				require.Equal(t, "env.literal", p.Keys[0].AzureKeyConfig.ClientSecret.GetValue())
			}
			if tc.name == system.OpenAIModelProvider {
				require.Equal(t, "env.literal", p.Keys[0].Value.GetValue())
			}
			if tc.name == system.AmazonBedrockModelProvider {
				require.Equal(t, "session", p.Keys[0].BedrockMantleKeyConfig.SessionToken.GetValue())
				require.Equal(t, "eu-west-1", p.Keys[0].BedrockMantleKeyConfig.Region.GetValue())
			}
			if tc.name == system.AmazonBedrockAPIKeyModelProvider {
				require.Equal(t, "us-east-1", p.Keys[0].BedrockMantleKeyConfig.Region.GetValue())
			}
			if tc.name == system.GenericResponsesModelProvider {
				require.Equal(t, "https://example.com/prefix", p.Config.NetworkConfig.BaseURL)
			}
			_, err = legacyAggregate(t.Context(), tc.name, map[string]string{})
			require.Error(t, err)
		})
	}
}

func TestMigrationDoesNotOverwriteExistingProvider(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "legacy"})
	require.NoError(t, e.store.CreateProvider(ctx, "openai", llmcoreclient.Provider{Keys: []schemas.Key{{ID: "manual", Name: "manual", Value: *schemas.NewSecretVar("manual")}}}))
	results, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	require.Equal(t, "blocked", results[0].Status)
	p, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.Equal(t, "manual", p.Keys[0].Value.GetValue())
	require.Nil(t, p.Migration)
	sources, err := e.migratedSources(ctx)
	require.NoError(t, err)
	require.Empty(t, sources)
}

func TestMigrationConcurrentImport(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
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
	p, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.Len(t, p.Keys, 1)
	require.Equal(t, system.OpenAIModelProvider, p.Migration.Source)
}

func TestMigrationRetriesResourceFailure(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
	failed := false
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
		if _, ok := obj.(*v1.LLMGatewayProvider); ok && !failed {
			failed = true
			return errors.New("injected provider failure")
		}
		return c.Create(ctx, obj, opts...)
	}}).Build()
	store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	e.store = store
	results, err := e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.ErrorContains(t, err, "injected provider failure")
	require.Equal(t, "error", results[0].Status)
	_, err = store.GetProvider(t.Context(), "openai")
	require.True(t, apierrors.IsNotFound(err))
	results, err = e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.NoError(t, err)
	require.Equal(t, "imported", results[0].Status)
}

func (*legacyCredentialTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return data, nil
}

func (t *legacyCredentialTransformer) TransformFromStorage(_ context.Context, data []byte, dataCtx value.Context) ([]byte, bool, error) {
	if t.fail && strings.HasPrefix(string(dataCtx.AuthenticatedData()), system.AzureEntraModelProvider+"///") {
		return nil, false, errors.New("injected credential read failure")
	}
	return data, false, nil
}

func TestMigrationCredentialReadFailureDoesNotBlockOtherProviders(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	storage, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(storage.DB.DB, storage.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	transformer := &legacyCredentialTransformer{fail: true}
	g := gatewayclient.New(ctx, db, nil, &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			{Group: "obot.obot.ai", Resource: "credentials"}: transformer,
		},
	}, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = g.Close() })
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
	putLegacy(t, g, system.AzureModelProvider, azureSecrets("https://one.openai.azure.com"))
	putLegacy(t, g, system.AzureEntraModelProvider, map[string]string{
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT":      "https://two.services.ai.azure.com",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID":     "client",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret",
		"OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID":     "tenant",
	})
	migrationDNS(t)
	_, err = e.ImportLegacyModelProviders(ctx, nil, g)
	require.ErrorContains(t, err, "injected credential read failure")
	p, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err, "an unreadable Azure credential must not block OpenAI")
	require.Len(t, p.Keys, 1)
	_, err = e.store.GetProvider(ctx, "azure")
	require.True(t, apierrors.IsNotFound(err), "an unreadable family member must not leave a partial pool")
	status, err := e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "error", status.Sources[system.AzureEntraModelProvider].Status)

	transformer.fail = false
	_, err = e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	p, err = e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Len(t, p.Keys, 2)
	status, err = e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "imported", status.Sources[system.AzureEntraModelProvider].Status)
}

func TestMigrationSettingsConflictPreservesOtherSettings(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
	conflict := true
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
		if _, ok := obj.(*v1.LLMGatewayConfig); ok && conflict {
			conflict = false
			return apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayconfigs"}, obj.GetName(), errors.New("injected conflict"))
		}
		return c.Update(ctx, obj, opts...)
	}}).Build()
	store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	e := New()
	e.store = store
	require.NoError(t, store.MutateSettings(t.Context(), func(s map[string]json.RawMessage) error { s["unrelated"] = json.RawMessage(`true`); return nil }))
	_, err = e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.NoError(t, err)
	require.False(t, conflict)
	settings, err := store.GetSettings(t.Context())
	require.NoError(t, err)
	require.JSONEq(t, `true`, string(settings["unrelated"]))
	state, err := e.MigrationStatus(t.Context())
	require.NoError(t, err)
	require.Equal(t, "imported", state.Sources[system.OpenAIModelProvider].Status)
}

func TestMigrationReferenceRecoveryPreservesLaterEdits(t *testing.T) {
	ns := system.DefaultNamespace
	legacy := &v1.Model{Namespace: ns, Name: "m1-old", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.OpenAIModelProvider, TargetModel: "gpt-5", Active: false, Alias: "custom"}}}
	target := &v1.Model{Namespace: ns, Name: "m1-new", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "openai/gpt-5", Active: true}}}
	alias := &v1.DefaultModelAlias{Namespace: ns, Name: "llm", Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: legacy.Name}}}
	failDelete := true
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(legacy, target, alias).WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string { return []string{o.(*v1.Model).Spec.Manifest.ModelProvider} }).WithInterceptorFuncs(interceptor.Funcs{Delete: func(ctx context.Context, c kclient.WithWatch, o kclient.Object, opts ...kclient.DeleteOption) error {
		if o.GetName() == legacy.Name && failDelete {
			failDelete = false
			return errors.New("injected delete failure")
		}
		return c.Delete(ctx, o, opts...)
	}}).Build()
	ctx := t.Context()
	sources := map[string]string{system.OpenAIModelProvider: "openai"}
	require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, nil, func(map[string]string) error { return errors.New("injected settings failure") }), "injected settings failure")
	got := &v1.DefaultModelAlias{}
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(alias), got))
	require.Equal(t, legacy.Name, got.Spec.Manifest.Model)
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(target), target))
	require.False(t, target.Spec.Manifest.Active)
	// An administrator changes the target after its edits committed but before cleanup.
	target.Spec.Manifest.Active = true
	target.Spec.Manifest.Alias = "new-choice"
	require.NoError(t, c.Update(ctx, target))
	mappings := map[string]string{}
	save := func(m map[string]string) error {
		maps.Copy(mappings, m)
		return nil
	}
	require.ErrorContains(t, migrateLegacyModels(ctx, c, sources, nil, save), "injected delete failure")
	require.Equal(t, target.Name, mappings[legacy.Name])
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(alias), got))
	require.Equal(t, target.Name, got.Spec.Manifest.Model)
	require.NoError(t, migrateLegacyModels(ctx, c, sources, nil, save))
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(target), target))
	require.True(t, target.Spec.Manifest.Active)
	require.Equal(t, "new-choice", target.Spec.Manifest.Alias)
	require.True(t, apierrors.IsNotFound(c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})))
}

func TestMigrationDoesNotMoveUnverifiedWildcardGrants(t *testing.T) {
	ns := system.DefaultNamespace
	models := []*v1.Model{
		{Namespace: ns, Name: "m1-api", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.AzureModelProvider, TargetModel: "gpt-5", Active: true}}},
		{Namespace: ns, Name: "m1-entra", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.AzureEntraModelProvider, TargetModel: "gpt-5", Active: false}}},
		{Namespace: ns, Name: "m1-gw", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "azure/gpt-5", Active: true}}},
	}
	policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Models: []types.ModelResource{{ID: "gpt*"}, {ID: "m1-entra"}, {ID: "m1-api"}}}}}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(models[0], models[1], models[2], policy).WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string { return []string{o.(*v1.Model).Spec.Manifest.ModelProvider} }).Build()
	require.NoError(t, migrateLegacyModels(t.Context(), c, map[string]string{system.AzureModelProvider: "azure"}, nil, func(map[string]string) error { return nil }))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: "gpt*"}, {ID: "azure/gpt*"}, {ID: "m1-entra"}, {ID: "m1-gw"}}, policy.Spec.Manifest.Models)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(models[2]), models[2]))
	require.True(t, models[2].Spec.Manifest.Active, "unverified inactive source must not disable the gateway target")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(models[1]), models[1]))
}

func TestMigrationWaitsForDiscoveryBeforeQualifyingWildcardGrants(t *testing.T) {
	ns := system.DefaultNamespace
	old := &v1.Model{Namespace: ns, Name: "m1-legacy", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.OpenAIModelProvider, TargetModel: "gpt-5", Active: true}}}
	target := &v1.Model{Namespace: ns, Name: "m1-target", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "openai/gpt-5", Active: true}}}
	policy := &v1.ModelAccessPolicy{Namespace: ns, Name: "policy", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{Models: []types.ModelResource{{ID: "gpt*"}}}}}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(old, target, policy).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).Build()
	sources := map[string]string{system.OpenAIModelProvider: "openai"}
	save := func(map[string]string) error { return nil }
	require.NoError(t, migrateLegacyModels(t.Context(), c, sources, map[string]bool{}, save))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: "gpt*"}}, policy.Spec.Manifest.Models, "an undiscovered target must not gain a gateway grant")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(old), &v1.Model{}))

	require.NoError(t, migrateLegacyModels(t.Context(), c, sources, map[string]bool{target.Spec.Manifest.TargetModel: true}, save))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: "openai/gpt*"}}, policy.Spec.Manifest.Models)
}

func TestMigrationDoesNotRecreateDeletedImportedProvider(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
	_, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	require.NoError(t, e.store.DeleteProvider(ctx, "openai"))
	for range 2 {
		results, err := e.ImportLegacyModelProviders(ctx, nil, g)
		require.NoError(t, err)
		require.Equal(t, "blocked", results[0].Status)
		_, err = e.store.GetProvider(ctx, "openai")
		require.True(t, apierrors.IsNotFound(err))
	}
}
func TestMigrationReportsUnsupportedSources(t *testing.T) {
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.Model{Namespace: system.DefaultNamespace, Name: "m1-unsupported", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: "ollama-model-provider", TargetModel: "llama3", Active: true}}}).Build()
	results, err := e.ImportLegacyModelProviders(ctx, c, g)
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "unsupported", results[0].Status)
	require.NoError(t, e.MigrateLegacyModels(ctx, c))
	require.NoError(t, c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: "m1-unsupported"}, &v1.Model{}))
}

func TestMigrationCodecFailure(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	codec := &failingMigrationCodec{fail: true}
	store, err := llmcoreclient.New(c, codec)
	require.NoError(t, err)
	e := New()
	e.store = store
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "must-not-be-written"})
	_, err = e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.ErrorContains(t, err, "injected codec failure")
	var providers v1.LLMGatewayProviderList
	require.NoError(t, c.List(t.Context(), &providers))
	require.Empty(t, providers.Items)
	state, err := e.MigrationStatus(t.Context())
	require.NoError(t, err)
	raw, _ := json.Marshal(state)
	require.NotContains(t, string(raw), "must-not-be-written")
	codec.fail = false
	_, err = e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.NoError(t, err)
	p, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, p.Keys, 1)
}

func TestMigrationWaitsForProviderDiscoveryBeforeUsingSubscriptionModels(t *testing.T) {
	ns := system.DefaultNamespace
	old := &v1.Model{Namespace: ns, Name: "m1-legacy", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.AnthropicModelProvider, TargetModel: "claude-haiku-4-5", Active: true}}}
	catalog := &v1.Model{Namespace: ns, Name: "m1-catalog", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "anthropic/claude-haiku-4-5", Active: true}}}
	dated := &v1.Model{Namespace: ns, Name: "m1-dated", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "anthropic/claude-haiku-4-5-20251001", Active: true}}}
	alias := &v1.DefaultModelAlias{Namespace: ns, Name: "llm-mini", Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm-mini", Model: old.Name}}}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(old, catalog, dated, alias).WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string { return []string{o.(*v1.Model).Spec.Manifest.ModelProvider} }).Build()
	sources := map[string]string{system.AnthropicModelProvider: "anthropic"}
	save := func(map[string]string) error { return nil }
	require.NoError(t, migrateLegacyModels(t.Context(), c, sources, map[string]bool{}, save))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(old), &v1.Model{}))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(alias), alias))
	require.Equal(t, old.Name, alias.Spec.Manifest.Model)
	require.NoError(t, migrateLegacyModels(t.Context(), c, sources, map[string]bool{dated.Spec.Manifest.TargetModel: true}, save))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(alias), alias))
	require.Equal(t, dated.Name, alias.Spec.Manifest.Model, "a synthetic exact name must not beat the provider's discovered dated version")
}

func TestMigrationDiscoveryRejectsCoreCatalogFallback(t *testing.T) {
	e, ctx := startTestEngine(t)
	path, _ := modeldata.Paths(e.opts.DataDir)
	require.NoError(t, os.WriteFile(path, []byte(`{"claude-haiku-4-5":{"provider":"anthropic","mode":"chat"}}`), 0600))
	require.NoError(t, e.prices.ForceReloadPricing(ctx))
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"unavailable"}}`))
	}))
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "anthropic", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true, "max_retries": 0}}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "anthropic", testJSON(t, map[string]any{"name": "test", "value": "key"}))))
	_, err := e.ListModels(ctx)
	require.NoError(t, err)
	require.Empty(t, e.availability.discoveredModels(), "catalog fallback must not count as a successful provider listing")
	// Use a second upstream to supply a real response, without changing the source's protocol.
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-haiku-4-5-20251001","type":"model"}],"has_more":false}`))
	}))
	t.Cleanup(live.Close)
	require.NoError(t, e.store.MutateProvider(ctx, "anthropic", func(p *llmcoreclient.Provider) error { p.Config.NetworkConfig.BaseURL = live.URL; return nil }))
	_, err = e.ListModels(ctx)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"anthropic/claude-haiku-4-5-20251001": true}, e.availability.discoveredModels())
}
