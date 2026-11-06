package llmengine

import (
	"context"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCoreSyncPicksUpAnotherReplicasChanges(t *testing.T) {
	ctx := t.Context()
	opts := testOptions(t, Options{DataDir: t.TempDir()})
	a, b := New(), New()
	for _, e := range []*Engine{a, b} {
		if err := e.Start(ctx, opts); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	}
	if err := testMutation(ctx, a, nil)(a.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai"}))); err != nil {
		t.Fatal(err)
	}
	before := b.core.Load()
	changed, err := b.Sync(ctx)
	if err != nil || !changed {
		t.Fatalf("sync = %t, %v", changed, err)
	}
	if b.core.Load() != before {
		t.Fatal("sync rebuilt the core engine")
	}
	if changed, err := b.Sync(ctx); err != nil || changed {
		t.Fatalf("unchanged sync = %t, %v", changed, err)
	}
}

func TestListModelsSyncsAnotherReplicasChanges(t *testing.T) {
	ctx := t.Context()
	opts := testOptions(t, Options{DataDir: t.TempDir()})
	a, b := New(), New()
	for _, e := range []*Engine{a, b} {
		require.NoError(t, e.Start(ctx, opts))
		t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	}
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, a, nil)(a.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(ctx, a, nil)(a.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "secret", "models": []string{"*"}}))))
	providers, err := b.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Empty(t, providers, "replica B has not reconciled yet")
	models, err := b.ListModels(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, models.Models, "the triggered sync must list the newly configured provider immediately")
	providers, err = b.Client().GetConfiguredProviders()
	require.NoError(t, err)
	require.Contains(t, providers, schemas.OpenAI)
}

func TestListModelsSurvivesPartialSyncFailure(t *testing.T) {
	e, ctx := startTestEngine(t)
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateProvider(ctx, testJSON(t, map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}))))
	require.NoError(t, testMutation(ctx, e, nil)(e.CreateKey(ctx, "openai", testJSON(t, map[string]any{"name": "test", "value": "secret", "models": []string{"*"}}))))
	// Simulate a persisted configuration this replica cannot fully apply.
	require.NoError(t, e.store.CreateProvider(ctx, "broken", llmcoreclient.Provider{}))
	for range 2 {
		models, err := e.ListModels(ctx)
		require.NoError(t, err, "a failed provider update must not hide healthy models")
		require.NotEmpty(t, models.Models)
		ids := make([]string, 0, len(models.Models))
		for _, model := range models.Models {
			ids = append(ids, model.ID)
		}
		require.Contains(t, ids, "openai/gpt-4o-mini")
		require.Error(t, e.Err(), "the synchronization failure remains visible in status")
	}
}

func TestDegradedStartupExposesProviderDiagnostics(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	for _, name := range []string{"openai", "anthropic"} {
		require.NoError(t, store.CreateProvider(ctx, name, llmcoreclient.Provider{}))
	}
	var providers v1.LLMGatewayProviderList
	require.NoError(t, storage.List(ctx, &providers))
	var corrupt *v1.LLMGatewayProvider
	for i := range providers.Items {
		if providers.Items[i].Spec.ID == "anthropic" {
			corrupt = &providers.Items[i]
		}
	}
	require.NotNil(t, corrupt)
	original := corrupt.Spec.Payload
	corrupt.Spec.Payload = "invalid JSON"
	require.NoError(t, storage.Update(ctx, corrupt))
	engine := New()
	require.NoError(t, engine.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })
	require.True(t, engine.Ready(), "healthy providers must remain usable")
	require.ErrorContains(t, engine.Err(), "anthropic", "startup must expose core diagnostics before a watch event")
	var configured map[string]any
	require.NoError(t, testResult(&configured, "")(engine.GetProvider(ctx, "openai")))
	require.Equal(t, "openai", configured["name"])
	_, err = engine.Sync(ctx)
	require.ErrorContains(t, err, "anthropic")
	corrupt.Spec.Payload = original
	require.NoError(t, storage.Update(ctx, corrupt))
	_, err = engine.Sync(ctx)
	require.NoError(t, err)
	require.NoError(t, engine.Err(), "repair must clear the replica-local error")
}
