package llmengine

import (
	"slices"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
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
	gatewayClient := gatewayclient.New(ctx, db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
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
			want = "skipped"
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
