package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type migrationSnapshotCodec struct {
	plaintextConfigCodec
	opens atomic.Int64
	fail  atomic.Bool
}

func (c *migrationSnapshotCodec) OpenGatewayConfig(ctx context.Context, id string, data []byte) ([]byte, error) {
	if id == "settings" {
		c.opens.Add(1)
		if c.fail.Load() {
			return nil, errors.New("settings decode failed")
		}
	}
	return c.plaintextConfigCodec.OpenGatewayConfig(ctx, id, data)
}

func TestMigrationSnapshotRefreshAndReplicas(t *testing.T) {
	ctx := t.Context()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	writer, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	set := func(raw string) {
		t.Helper()
		require.NoError(t, writer.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			if raw == "" {
				delete(settings, migrationSetting)
			} else {
				settings[migrationSetting] = json.RawMessage(raw)
			}
			return nil
		}))
	}
	start := func() *Engine {
		t.Helper()
		// Each replica has its own resource client and applied configuration.
		store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
		require.NoError(t, err)
		e := New()
		require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
		t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
		return e
	}
	lookup := func(e *Engine, id, want string) {
		t.Helper()
		got, err := e.MigratedReference(ctx, id)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
	lookup(New(), "old", "")
	set(`{"mappings":{"old":"target","removed":"gone"}}`)
	a, b := start(), start()
	for _, e := range []*Engine{a, b} {
		lookup(e, " old ", "target")
	}
	set(`{"mappings":{"old":"updated","added":"new"}}`)
	state, err := a.MigrationStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, "updated", state.Mappings["old"], "admin reads remain authoritative before reconciliation")
	state.Mappings["old"] = "caller mutation"
	for _, e := range []*Engine{a, b} {
		lookup(e, "old", "target")
		_, err = e.Sync(ctx)
		require.NoError(t, err)
		lookup(e, "old", "updated")
		lookup(e, "added", "new")
		lookup(e, "removed", "")
	}
	lookup(start(), "old", "updated")
	for _, raw := range []string{`{"mappings":{}}`, ""} {
		set(raw)
		for _, e := range []*Engine{a, b} {
			_, err = e.Sync(ctx)
			require.NoError(t, err)
			lookup(e, "old", "")
			lookup(e, "added", "")
		}
	}
}

func TestMigrationSnapshotFailuresAndRequestReads(t *testing.T) {
	ctx := t.Context()
	var reads atomic.Int64
	var failRead atomic.Bool
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			if _, ok := obj.(*v1.LLMGatewayConfig); ok {
				reads.Add(1)
				if failRead.Load() {
					return errors.New("settings read failed")
				}
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	codec := &migrationSnapshotCodec{}
	store, err := llmcoreclient.New(storage, codec)
	require.NoError(t, err)
	set := func(raw string) {
		t.Helper()
		require.NoError(t, store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			settings[migrationSetting] = json.RawMessage(raw)
			return nil
		}))
	}
	set(`{"mappings":{"old":"target"}}`)
	e := New()
	require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })

	failRead.Store(true)
	codec.fail.Store(true)
	beforeReads, beforeOpens := reads.Load(), codec.opens.Load()
	for range 100 {
		for _, id := range []string{"old", "openai/canonical", "missing"} {
			got, err := e.MigratedReference(ctx, id)
			require.NoError(t, err)
			want := ""
			if id == "old" {
				want = "target"
			}
			require.Equal(t, want, got)
		}
	}
	require.Equal(t, beforeReads, reads.Load(), "lookup must never read settings")
	require.Equal(t, beforeOpens, codec.opens.Load(), "lookup must never decode settings")
	accepted := e.migratedMappings.Load()
	_, err = e.Sync(ctx)
	require.ErrorContains(t, err, "settings read failed")
	require.Same(t, accepted, e.migratedMappings.Load(), "a failed read must not republish a snapshot")
	failRead.Store(false)
	_, err = e.Sync(ctx)
	require.ErrorContains(t, err, "settings decode failed")
	require.Same(t, accepted, e.migratedMappings.Load())
	codec.fail.Store(false)

	for _, raw := range []string{
		`{"mappings":{"old":"replacement","partial":"new","invalid":123}}`,
		`{"mappings":{"old":"replacement"},"sources":[]}`,
	} {
		set(raw)
		_, err = e.Sync(ctx)
		require.Error(t, err)
		require.Same(t, accepted, e.migratedMappings.Load(), "malformed state must retain the accepted snapshot")
		require.Error(t, e.Err(), "malformed metadata must remain visible in replica status")
		require.True(t, e.Ready(), "healthy configuration remains usable, like other sync failures")
		got, err := e.MigratedReference(ctx, "old")
		require.NoError(t, err)
		require.Equal(t, "target", got)
		got, err = e.MigratedReference(ctx, "partial")
		require.NoError(t, err)
		require.Empty(t, got, "do not publish partially decoded mappings")
	}
	set(`{"mappings":{}}`)
	_, err = e.Sync(ctx)
	require.NoError(t, err)
	require.NoError(t, e.Err())
	got, err := e.MigratedReference(ctx, "old")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestMigrationSnapshotMalformedStartup(t *testing.T) {
	ctx := t.Context()
	opts := testOptions(t, Options{DataDir: t.TempDir()})
	require.NoError(t, opts.Store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
		settings[migrationSetting] = json.RawMessage(`{"mappings":{"partial":"target","invalid":[]}}`)
		return nil
	}))
	e := New()
	require.NoError(t, e.Start(ctx, opts))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	require.Error(t, e.Err())
	got, err := e.MigratedReference(ctx, "partial")
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestMigrationSnapshotAdvancesWithHealthySettings(t *testing.T) {
	e, ctx := startTestEngine(t)
	// Core applies settings even when an unrelated provider needs another retry.
	require.NoError(t, e.store.CreateProvider(ctx, "broken", llmcoreclient.Provider{}))
	for _, target := range []string{"updated", ""} {
		require.NoError(t, e.store.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			mappings := map[string]string{}
			if target != "" {
				mappings["old"] = target
			}
			settings[migrationSetting] = testJSON(t, MigrationStatus{Mappings: mappings})
			return nil
		}))
		_, err := e.Sync(ctx)
		require.Error(t, err, "the independent provider error remains observable")
		got, err := e.MigratedReference(ctx, "old")
		require.NoError(t, err)
		require.Equal(t, target, got, "successfully loaded settings advance with healthy configuration")
	}
}

func TestMigrationSnapshotConcurrentSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	writer, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	set := func(target string) {
		t.Helper()
		require.NoError(t, writer.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
			settings[migrationSetting] = testJSON(t, MigrationStatus{Mappings: map[string]string{"old": target}})
			return nil
		}))
	}
	var block atomic.Bool
	loaded, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayConfig); ok && block.CompareAndSwap(true, false) {
				// Hold an old resource read while a newer resource and sync arrive.
				close(loaded)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return err
		},
	})
	store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	set("initial")
	e := New()
	require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	var readers sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					got, err := e.MigratedReference(ctx, "old")
					if err != nil || (got != "initial" && got != "older" && got != "newest") {
						t.Errorf("concurrent lookup = %q, %v", got, err)
						return
					}
				}
			}
		})
	}
	defer func() { close(stop); readers.Wait() }()
	set("older")
	block.Store(true)
	first := make(chan error, 1)
	go func() { _, err := e.Sync(ctx); first <- err }()
	select {
	case <-loaded:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	set("newest")
	second := make(chan error, 1)
	go func() { _, err := e.Sync(ctx); second <- err }()
	unblock()
	for _, result := range []chan error{first, second} {
		select {
		case err := <-result:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatalf("sync did not finish: %v", ctx.Err())
		}
	}
	got, err := e.MigratedReference(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, "newest", got, "an older sync must never overwrite the newer snapshot")
}
