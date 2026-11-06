package llmengine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/require"
)

type blockedCleanupPlugin struct {
	schemas.LLMPlugin
	entered chan struct{}
	release chan struct{}
}

func (*blockedCleanupPlugin) GetName() string { return "blocked-cleanup" }

func (p *blockedCleanupPlugin) Cleanup() error {
	close(p.entered)
	<-p.release
	return nil
}

func TestShutdownBoundsWaitAndFinishesCleanup(t *testing.T) {
	plugin := &blockedCleanupPlugin{entered: make(chan struct{}), release: make(chan struct{})}
	engine := New()
	require.NoError(t, engine.Start(t.Context(), testOptions(t, Options{DataDir: t.TempDir(), LastPlugin: plugin})))
	var release sync.Once
	defer func() {
		release.Do(func() { close(plugin.release) })
		require.NoError(t, engine.Shutdown(context.Background()))
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	returned := make(chan error, 1)
	go func() { returned <- engine.Shutdown(ctx) }()
	select {
	case <-plugin.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("core cleanup did not start")
	}
	select {
	case err := <-returned:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown ignored its deadline")
	}
	select {
	case <-engine.shutdownDone:
		t.Fatal("catalog cleanup finished before core cleanup")
	default:
	}
	release.Do(func() { close(plugin.release) })
	wait, cancelWait := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelWait()
	require.NoError(t, engine.Shutdown(wait))
	require.NoError(t, engine.Shutdown(wait), "later callers join the same cleanup")
}

func TestCancellationStartsShutdown(t *testing.T) {
	plugin := &blockedCleanupPlugin{entered: make(chan struct{}), release: make(chan struct{})}
	engine := New()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, engine.Start(ctx, testOptions(t, Options{DataDir: t.TempDir(), LastPlugin: plugin})))
	defer func() {
		close(plugin.release)
		require.NoError(t, engine.Shutdown(context.Background()))
	}()

	cancel()
	select {
	case <-plugin.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not start core cleanup")
	}
	// An explicit application shutdown joins the cleanup started by cancellation.
	deadline, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	require.ErrorIs(t, engine.Shutdown(deadline), context.DeadlineExceeded)
	select {
	case <-engine.shutdownDone:
		t.Fatal("cleanup finished before the blocked plugin was released")
	default:
	}
}

func TestShutdownJoinsBackgroundStartup(t *testing.T) {
	plugin := &blockedCleanupPlugin{entered: make(chan struct{}), release: make(chan struct{})}
	engine := New()
	deadline, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	require.ErrorIs(t, engine.Shutdown(deadline), context.DeadlineExceeded)

	// Simulate the application asking for shutdown before its startup goroutine runs.
	require.NoError(t, engine.Start(t.Context(), testOptions(t, Options{DataDir: t.TempDir(), LastPlugin: plugin})))
	defer func() {
		close(plugin.release)
		require.NoError(t, engine.Shutdown(context.Background()))
	}()
	select {
	case <-plugin.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown missed the engine created by background startup")
	}
}
