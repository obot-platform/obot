package llmengine

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestLegacyImportLeavesEnvProviders checks that, on an upgrade, a provider whose API key
// variable is set gets only the key from the variable, not the legacy provider's credential as
// well, while other legacy credentials are imported.
func TestLegacyImportLeavesEnvProviders(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	t.Setenv(system.AnthropicAPIKeyEnvVar, "sk-ant-from-env-0123456789")
	e, ctx := startTestEngine(t)

	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gatewayClient := gatewayclient.New(ctx, db, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	for name, envVar := range map[string]string{
		system.OpenAIModelProvider:    "OBOT_OPENAI_MODEL_PROVIDER_API_KEY",
		system.AnthropicModelProvider: "OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY",
	} {
		if err := gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: name,
			Name:    name,
			Secrets: map[string]string{envVar: "legacy-" + name},
		}); err != nil {
			t.Fatal(err)
		}
	}

	results, err := e.ImportLegacyModelProviders(ctx, nil, gatewayClient)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		want := "imported"
		if r.GatewayProvider == "anthropic" {
			want = "environment"
		}
		if r.Status != want {
			t.Errorf("%s: status %s (%s), want %s", r.ModelProvider, r.Status, r.Message, want)
		}
	}

	mp := ModelProviderResource()
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
	if !e.importEnvProviders(ctx, c, mp) {
		t.Fatal("expected the Anthropic provider to be configured from the environment")
	}
	for provider, want := range map[string]string{
		"openai":    "imported-" + system.OpenAIModelProvider,
		"anthropic": "env-" + system.AnthropicAPIKeyEnvVar,
	} {
		keys, err := e.providerKeys(ctx, provider)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, k := range keys {
			name, _ := k["name"].(string)
			names = append(names, name)
		}
		if !slices.Equal(names, []string{want}) {
			t.Errorf("%s has keys %v, want [%s]", provider, names, want)
		}
	}
}

// Old startup markers survive a switch of configuration storage. They are only
// a cache: the matching resource and named key must exist before skipping work.
func TestEnvImportRepairsStaleMarkers(t *testing.T) {
	const value = "current-environment-key"
	t.Setenv(system.OpenAIAPIKeyEnvVar, value)
	t.Setenv(system.AnthropicAPIKeyEnvVar, "")
	for _, tc := range []struct {
		name         string
		existingKeys []schemas.Key
	}{
		{
			name: "missing provider resource",
		},
		{
			name:         "missing environment key",
			existingKeys: []schemas.Key{{ID: "manual", Name: "manual", Value: *schemas.NewSecretVar("manual-key")}},
		},
		{
			name:         "outdated environment key",
			existingKeys: []schemas.Key{{ID: "environment", Name: "env-" + system.OpenAIAPIKeyEnvVar, Value: *schemas.NewSecretVar("old-key")}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			if tc.existingKeys != nil {
				require.NoError(t, e.store.CreateProvider(ctx, "openai", llmcoreclient.Provider{Keys: tc.existingKeys}))
			}
			sum := sha256.Sum256([]byte(value))
			mp := ModelProviderResource()
			mp.Annotations = map[string]string{"obot.ai/llm-gateway-env-key-openai": hex.EncodeToString(sum[:])}
			c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(mp).Build()
			require.True(t, e.importEnvProviders(ctx, c, mp), "a hash marker alone must not hide missing configuration")
			provider, err := e.store.GetProvider(ctx, "openai")
			require.NoError(t, err)
			index := slices.IndexFunc(provider.Keys, func(k schemas.Key) bool { return k.Name == "env-"+system.OpenAIAPIKeyEnvVar })
			require.GreaterOrEqual(t, index, 0)
			require.Equal(t, value, provider.Keys[index].Value.GetValue())
			if tc.name == "missing environment key" {
				require.Len(t, provider.Keys, 2)
				require.Equal(t, "manual-key", provider.Keys[0].Value.GetValue())
			}
			require.False(t, e.importEnvProviders(ctx, c, mp), "an intact resource should not be rewritten on every startup")
		})
	}
}
