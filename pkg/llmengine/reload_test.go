package llmengine

import (
	"net/http"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoretypes "github.com/obot-platform/obot/pkg/llmcore/types"
	"github.com/stretchr/testify/require"
)

func TestCoreSyncPicksUpAnotherReplicasChanges(t *testing.T) {
	ctx := t.Context()
	opts := testOptions(t, Options{DataDir: t.TempDir()})
	a, b := New(), New()
	for _, e := range []*Engine{a, b} {
		if err := e.Start(ctx, opts); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(e.Shutdown)
	}
	if err := a.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai"}, nil); err != nil {
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
		t.Cleanup(e.Shutdown)
	}
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	before := b.core.Load().Revision()
	require.NoError(t, a.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}, nil))
	require.NoError(t, a.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "test", "value": "secret", "models": []string{"*"}}, nil))
	require.Equal(t, before, b.core.Load().Revision(), "replica B has not polled yet")
	models, err := b.ListModels(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, models.Models, "the triggered sync must list the newly configured provider immediately")
	rev, err := opts.Store.Revision(ctx)
	require.NoError(t, err)
	require.Equal(t, rev, b.core.Load().Revision())
}

func TestListModelsSurvivesPartialSyncFailure(t *testing.T) {
	e, ctx := startTestEngine(t)
	up := mockOpenAI(t)
	t.Cleanup(up.Close)
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": "openai", "network_config": map[string]any{"base_url": up.URL, "allow_private_network": true}}, nil))
	require.NoError(t, e.Do(ctx, http.MethodPost, "/api/providers/openai/keys", map[string]any{"name": "test", "value": "secret", "models": []string{"*"}}, nil))
	// Simulate a persisted configuration this replica cannot fully apply.
	require.NoError(t, e.store.Mutate(ctx, func(cfg *llmcoretypes.Config) error {
		cfg.Providers["broken"] = schemas.ProviderConfig{}
		return nil
	}))
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
