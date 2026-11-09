package llmengine

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/apiclient/types"
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

func TestLegacyGenericResponsesOptionalKey(t *testing.T) {
	for _, apiKey := range []string{"", "synthetic-stored-key"} {
		t.Run(map[bool]string{true: "keyless", false: "stored key"}[apiKey == ""], func(t *testing.T) {
			secrets := map[string]string{
				"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://8.8.8.8/prefix/v1/",
				"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY":  apiKey,
			}
			p, err := legacyAggregate(t.Context(), system.GenericResponsesModelProvider, secrets)
			require.NoError(t, err)
			require.Equal(t, "https://8.8.8.8/prefix", p.Config.NetworkConfig.BaseURL)
			require.Equal(t, system.GenericResponsesModelProvider, p.Migration.Source)
			require.Equal(t, schemas.OpenAI, p.Config.CustomProviderConfig.BaseProviderType)
			require.Equal(t, apiKey == "", p.Config.CustomProviderConfig.IsKeyLess)
			if apiKey == "" {
				require.Empty(t, p.Keys, "keyless imports must not manufacture a credential")
				delete(secrets, "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY")
				missing, err := legacyAggregate(t.Context(), system.GenericResponsesModelProvider, secrets)
				require.NoError(t, err)
				require.Equal(t, p, missing)
			} else {
				require.Len(t, p.Keys, 1)
				require.Equal(t, apiKey, p.Keys[0].Value.GetValue())
				require.Equal(t, "plain_text", string(p.Keys[0].Value.Type()))
			}
			delete(secrets, "OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL")
			_, err = legacyAggregate(t.Context(), system.GenericResponsesModelProvider, secrets)
			require.ErrorContains(t, err, "missing _MODEL_PROVIDER_BASE_URL")
		})
	}
}

func TestMigrationKeylessSourceVerification(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		provider   string
		provenance string
		base       schemas.ModelProvider
		keyless    bool
		approved   bool
	}{
		{
			name:       "verified Generic Responses",
			source:     system.GenericResponsesModelProvider,
			provider:   "generic-responses",
			provenance: system.GenericResponsesModelProvider,
			base:       schemas.OpenAI,
			keyless:    true,
			approved:   true,
		},
		{
			name:     "manual keyless provider",
			source:   system.GenericResponsesModelProvider,
			provider: "generic-responses",
			base:     schemas.OpenAI,
			keyless:  true,
		},
		{
			name:       "wrong provenance",
			source:     system.GenericResponsesModelProvider,
			provider:   "generic-responses",
			provenance: system.OpenAIModelProvider,
			base:       schemas.OpenAI,
			keyless:    true,
		},
		{
			name:       "missing key without explicit keyless config",
			source:     system.GenericResponsesModelProvider,
			provider:   "generic-responses",
			provenance: system.GenericResponsesModelProvider,
			base:       schemas.OpenAI,
		},
		{
			name:       "wrong base provider",
			source:     system.GenericResponsesModelProvider,
			provider:   "generic-responses",
			provenance: system.GenericResponsesModelProvider,
			base:       schemas.Anthropic,
			keyless:    true,
		},
		{
			name:       "unrelated custom provider",
			source:     system.GenericResponsesModelProvider,
			provider:   "unrelated",
			provenance: system.GenericResponsesModelProvider,
			base:       schemas.OpenAI,
			keyless:    true,
		},
		{
			name:       "builtin without keys",
			source:     system.OpenAIModelProvider,
			provider:   "openai",
			provenance: system.OpenAIModelProvider,
		},
		{
			name:       "other source cannot claim keyless import",
			source:     system.OpenAIModelProvider,
			provider:   "generic-responses",
			provenance: system.OpenAIModelProvider,
			base:       schemas.OpenAI,
			keyless:    true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			e.store = legacyImportInterceptStore(t, interceptor.Funcs{})
			p := llmcoreclient.Provider{}
			if tc.base != "" {
				p.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: tc.base, IsKeyLess: tc.keyless}
			}
			if tc.provenance != "" {
				p.Migration = &llmcoreclient.ProviderMigration{Source: tc.provenance}
			}
			require.NoError(t, e.store.CreateProvider(t.Context(), tc.provider, p))
			require.NoError(t, e.mutateMigration(t.Context(), func(s *MigrationStatus) error {
				s.Sources[tc.source] = MigrationSource{Provider: tc.provider, Status: "imported", Imported: true}
				return nil
			}))
			sources, err := e.migratedSources(t.Context())
			require.NoError(t, err)
			_, approved := sources[tc.source]
			require.Equal(t, tc.approved, approved)
		})
	}
}

func TestLegacyGenericResponsesRequiresDestination(t *testing.T) {
	for _, address := range []string{"", "/v1", "////", "http://", " ", "file:///v1", "http://10.1.2.3/v1", "http://169.254.169.254/v1"} {
		for _, apiKey := range []string{"", "synthetic-stored-key"} {
			t.Run(address+"/"+map[bool]string{true: "keyless", false: "stored key"}[apiKey == ""], func(t *testing.T) {
				_, err := legacyAggregate(t.Context(), system.GenericResponsesModelProvider, map[string]string{
					"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": address,
					"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_API_KEY":  apiKey,
				})
				require.Error(t, err)
				require.NotContains(t, err.Error(), "synthetic-stored-key")
			})
		}
	}
}

func TestMigratedGenericResponsesNoAuthorization(t *testing.T) {
	// Clamp ambient credentials before startup and exercise fallback with synthetic values only.
	t.Setenv(system.OpenAIAPIKeyEnvVar, "synthetic-ambient-openai")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	e, ctx := startTestEngine(t)
	var mu sync.Mutex
	seen := map[string]int{}
	var authorization []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path]++
		authorization = append(authorization, r.Header.Values("Authorization")...)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"test-model","object":"model"}]}`))
		case "/v1/responses":
			_, _ = w.Write([]byte(`{"id":"resp_test","object":"response","status":"completed","model":"test-model","output":[{"type":"message","id":"m1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"keyless response","annotations":[]}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	// A local fake proxy serves the public destination without bypassing import address policy.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "generic-migration.invalid:80" {
			http.Error(w, "unexpected proxy target", http.StatusBadRequest)
			return
		}
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(upstream.URL, "http://"), 5*time.Second)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer conn.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			t.Error(err)
			return
		}
		go func() { _, _ = io.Copy(conn, buffered) }()
		_, _ = io.Copy(client, conn)
	}))
	t.Cleanup(proxy.Close)
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(name, "")
	}
	t.Setenv("HTTP_PROXY", proxy.URL)
	g := migrationCredentials(t)
	putLegacy(t, g, system.GenericResponsesModelProvider, map[string]string{
		"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "http://generic-migration.invalid/v1",
	})
	legacy := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-keyless", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.GenericResponsesModelProvider, TargetModel: "test-model", Active: true}}}
	target := &v1.Model{Namespace: system.DefaultNamespace, Name: "m1-gateway", Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: system.LLMGatewayModelProvider, TargetModel: "generic-responses/test-model", Active: true}}}
	alias := &v1.DefaultModelAlias{Namespace: system.DefaultNamespace, Name: "llm", Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: legacy.Name}}}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(legacy, target, alias).WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string { return []string{o.(*v1.Model).Spec.Manifest.ModelProvider} }).Build()
	for range 2 {
		results, err := e.ImportLegacyModelProviders(ctx, c, g)
		require.NoError(t, err)
		require.Equal(t, "imported", results[0].Status)
	}
	p, err := e.store.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	require.Empty(t, p.Keys)
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	require.True(t, e.HasUpstreamProviders(ctx))
	require.NoError(t, e.MigrateLegacyModels(ctx, c))
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{}), "cleanup waits for discovery")
	models, err := e.listGatewayModels(ctx, "generic-responses", true)
	require.NoError(t, err)
	require.NotEmpty(t, models)
	code, body := do(t, e, http.MethodPost, "/v1/responses", `{"model":"generic-responses/test-model","input":"hello"}`)
	require.Equal(t, http.StatusOK, code, body)
	require.Contains(t, body, "keyless response")
	// Caller credentials remain subject to the existing rejection policy.
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"generic-responses/test-model","input":"hello"}`))
	req.Header.Set("Authorization", "Bearer synthetic-caller-key")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.NoError(t, e.MigrateLegacyModels(ctx, c))
	err = c.Get(ctx, kclient.ObjectKeyFromObject(legacy), &v1.Model{})
	require.True(t, apierrors.IsNotFound(err))
	require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(alias), alias))
	require.Equal(t, target.Name, alias.Spec.Manifest.Model)
	mapped, err := e.MigratedReference(ctx, legacy.Name)
	require.NoError(t, err)
	require.Equal(t, target.Name, mapped)
	mu.Lock()
	defer mu.Unlock()
	require.Positive(t, seen["/v1/models"])
	require.Equal(t, 1, seen["/v1/responses"])
	require.Empty(t, authorization, "neither ambient nor caller credentials may reach the keyless upstream")
}

func TestMigrationKeylessExistingProviderWins(t *testing.T) {
	e, ctx := startTestEngine(t)
	g := migrationCredentials(t)
	putLegacy(t, g, system.GenericResponsesModelProvider, map[string]string{
		"OBOT_GENERIC_RESPONSES_MODEL_PROVIDER_BASE_URL": "https://8.8.8.8/v1",
	})
	manual := llmcoreclient.Provider{Config: schemas.ProviderConfig{
		CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI, IsKeyLess: true},
		NetworkConfig:        schemas.NetworkConfig{BaseURL: "https://8.8.4.4", MaxRetries: 7},
	}}
	require.NoError(t, e.store.CreateProvider(ctx, "generic-responses", manual))
	before, err := e.store.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	results, err := e.ImportLegacyModelProviders(ctx, nil, g)
	require.NoError(t, err)
	require.Equal(t, "blocked", results[0].Status)
	after, err := e.store.GetProvider(ctx, "generic-responses")
	require.NoError(t, err)
	require.Equal(t, before, after)
	sources, err := e.migratedSources(ctx)
	require.NoError(t, err)
	require.Empty(t, sources)
}
