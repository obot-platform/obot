package llmengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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

func TestMigrationAppliesEgressProxyBeforePublishingProvider(t *testing.T) {
	azure := map[string]string{"OBOT_AZURE_MODEL_PROVIDER_API_KEY": "key", "OBOT_AZURE_MODEL_PROVIDER_ENDPOINT": "https://migration.openai.azure.com"}
	entra := map[string]string{"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT": "https://migration.services.ai.azure.com", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID": "client", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID": "tenant"}
	iam := map[string]string{"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID": "access", "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret", "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION": "eu-west-1"}
	apiKey := map[string]string{"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY": "key", "OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_REGION": "eu-west-1"}
	for _, tc := range []struct {
		source   string
		other    string
		family   string
		secrets  map[string]string
		otherKey map[string]string
		host     string
	}{
		{
			source:   system.AzureModelProvider,
			other:    system.AzureEntraModelProvider,
			family:   "azure",
			secrets:  azure,
			otherKey: entra,
			host:     "migration.openai.azure.com",
		},
		{
			source:   system.AzureEntraModelProvider,
			other:    system.AzureModelProvider,
			family:   "azure",
			secrets:  entra,
			otherKey: azure,
			host:     "migration.services.ai.azure.com",
		},
		{
			source:   system.AmazonBedrockModelProvider,
			other:    system.AmazonBedrockAPIKeyModelProvider,
			family:   "bedrock_mantle",
			secrets:  iam,
			otherKey: apiKey,
			host:     "bedrock-mantle.eu-west-1.api.aws",
		},
		{
			source:   system.AmazonBedrockAPIKeyModelProvider,
			other:    system.AmazonBedrockModelProvider,
			family:   "bedrock_mantle",
			secrets:  apiKey,
			otherKey: iam,
			host:     "bedrock-mantle.eu-west-1.api.aws",
		},
	} {
		t.Run(tc.source, func(t *testing.T) {
			for _, bypass := range []bool{false, true} {
				t.Run(map[bool]string{false: "proxied", true: "NO_PROXY"}[bypass], func(t *testing.T) {
					ctx := t.Context()
					created := false
					storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
							if resource, ok := obj.(*v1.LLMGatewayProvider); ok {
								var p llmcoreclient.Provider
								require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &p))
								if bypass {
									require.True(t, p.Config.ProxyConfig == nil || p.Config.ProxyConfig.Type == schemas.NoProxy)
								} else {
									require.NotNil(t, p.Config.ProxyConfig, "the first published version must already have its proxy")
									require.Equal(t, "http://egress.invalid:3128", p.Config.ProxyConfig.URL.GetValue())
								}
								created = true
							}
							return c.Create(ctx, obj, opts...)
						},
					}).Build()
					store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
					require.NoError(t, err)
					e := New()
					require.NoError(t, e.Start(ctx, testOptions(t, Options{Store: store, DataDir: t.TempDir()})))
					t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
					// Set after startup so the model catalog bootstrap cannot use the fake proxy.
					t.Setenv("HTTP_PROXY", "http://http-only.invalid:8080")
					t.Setenv("HTTPS_PROXY", "http://egress.invalid:3128")
					t.Setenv("NO_PROXY", map[bool]string{true: tc.host, false: "unrelated.invalid"}[bypass])
					g := migrationCredentials(t)
					putLegacy(t, g, tc.source, tc.secrets)
					putLegacy(t, g, tc.other, tc.otherKey)
					_, err = e.ImportLegacyModelProviders(ctx, nil, g)
					require.NoError(t, err)
					require.False(t, created, "cloud migration must wait for selection")
					status, err := e.MigrationStatus(ctx)
					require.NoError(t, err)
					require.NoError(t, e.ResolveMigration(ctx, tc.family, tc.source, status.Version))
					_, err = e.ImportLegacyModelProviders(ctx, nil, g)
					require.NoError(t, err)
					require.True(t, created)
					require.False(t, e.ApplyEgressProxy(ctx), "startup/admin proxy setup must agree with migration, including NO_PROXY")
					_, err = e.Sync(ctx)
					require.NoError(t, err)
					require.NotNil(t, e.Client().GetProviderByKey(schemas.ModelProvider(tc.family)), "selection must activate the provider in the running engine")
				})
			}
		})
	}
}

func TestMigratedAzureUsesEgressProxyWithoutRestart(t *testing.T) {
	e, ctx := startTestEngine(t)
	var mu sync.Mutex
	var paths []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o","object":"model"}]}`))
		} else {
			_, _ = w.Write([]byte(`{"id":"chat-1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"proxied"},"finish_reason":"stop"}]}`))
		}
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", "http://https-only.invalid:3128")
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	g := migrationCredentials(t)
	putLegacy(t, g, system.AzureModelProvider, azureSecrets("http://azure-migration.invalid"))
	putLegacy(t, g, system.AzureEntraModelProvider, map[string]string{"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT": "https://other.services.ai.azure.com", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID": "client", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret", "OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID": "tenant"})
	_, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	status, err := e.MigrationStatus(ctx)
	require.NoError(t, err)
	require.NoError(t, e.ResolveMigration(ctx, "azure", system.AzureModelProvider, status.Version))
	_, err = e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	models, err := e.listGatewayModels(ctx, schemas.Azure, true)
	require.NoError(t, err)
	require.NotEmpty(t, models)
	var response map[string]any
	require.NoError(t, testInference(t, e, ctx, http.MethodPost, "/v1/chat/completions", map[string]any{"model": "azure/gpt-4o", "messages": []any{map[string]any{"role": "user", "content": "hello"}}}, &response))
	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, paths, "/openai/v1/models")
	require.Contains(t, paths, "/openai/v1/chat/completions")
	require.Contains(t, response["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"], "proxied")
}

func TestMigrationRepairsEgressProxyAndPreservesExplicitSettings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		proxy   *schemas.ProxyConfig
		noProxy string
		want    *schemas.ProxyConfig
	}{
		{
			name: "repair a previously imported provider",
			want: &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://egress.invalid:3128")},
		},
		{
			name:    "leave an excluded endpoint direct",
			noProxy: ".openai.azure.com",
		},
		{
			name:    "remove an inherited proxy for NO_PROXY",
			proxy:   &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: &schemas.SecretVar{Val: "http://egress.invalid:3128"}},
			noProxy: ".openai.azure.com",
			want:    &schemas.ProxyConfig{Type: schemas.NoProxy},
		},
		{
			name:  "preserve an administrator proxy",
			proxy: &schemas.ProxyConfig{Type: schemas.Socks5Proxy, URL: schemas.NewSecretVar("socks5://admin.invalid:1080"), Username: schemas.NewSecretVar("user"), Password: schemas.NewSecretVar("password")},
			want:  &schemas.ProxyConfig{Type: schemas.Socks5Proxy, URL: schemas.NewSecretVar("socks5://admin.invalid:1080"), Username: schemas.NewSecretVar("user"), Password: schemas.NewSecretVar("password")},
		},
		{
			name:    "preserve an administrator proxy even for NO_PROXY",
			proxy:   &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://admin.invalid:8080")},
			noProxy: "*",
			want:    &schemas.ProxyConfig{Type: schemas.HTTPProxy, URL: schemas.NewSecretVar("http://admin.invalid:8080")},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			g := migrationCredentials(t)
			putLegacy(t, g, system.AzureModelProvider, azureSecrets("https://migration.openai.azure.com"))
			_, err := e.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			before.Config.ProxyConfig = tc.proxy
			before.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = 123
			require.NoError(t, e.store.MutateProvider(ctx, "azure", func(provider *llmcoreclient.Provider) error {
				provider.Config.ProxyConfig = before.Config.ProxyConfig
				provider.Config.NetworkConfig.DefaultRequestTimeoutInSeconds = before.Config.NetworkConfig.DefaultRequestTimeoutInSeconds
				return nil
			}))
			t.Setenv("HTTP_PROXY", "")
			t.Setenv("HTTPS_PROXY", "http://egress.invalid:3128")
			t.Setenv("NO_PROXY", tc.noProxy)
			t.Setenv("no_proxy", "")
			_, err = e.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			after, err := e.store.GetProvider(ctx, "azure")
			require.NoError(t, err)
			before.Config.ProxyConfig = tc.want
			require.Equal(t, before, after, "proxy inheritance must preserve keys, provenance and other provider settings")
			require.False(t, e.ApplyEgressProxy(ctx), "reconciliation must be idempotent")
		})
	}
}
