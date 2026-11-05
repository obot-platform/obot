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
