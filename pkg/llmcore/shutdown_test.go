package llmcore

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestShutdownBoundsWaitAndContinuesCleanup(t *testing.T) {
	for _, blockedOn := range []string{"reconciliation", "retirement"} {
		t.Run(blockedOn, func(t *testing.T) {
			store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
			require.NoError(t, err)
			engine, err := New(t.Context(), store, Options{})
			require.NoError(t, err)

			var release func()
			if blockedOn == "reconciliation" {
				engine.syncLock.Lock()
				release = engine.syncLock.Unlock
			} else {
				done := make(chan struct{})
				engine.retirements = map[schemas.ModelProvider]*providerRetirement{
					"pending": {done: done},
				}
				release = func() { close(done) }
			}
			var releaseOnce sync.Once
			defer func() {
				releaseOnce.Do(release)
				require.NoError(t, engine.Shutdown(context.Background()))
			}()

			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			returned := make(chan error, 1)
			go func() { returned <- engine.Shutdown(ctx) }()
			select {
			case err := <-returned:
				require.ErrorIs(t, err, context.DeadlineExceeded)
			case <-time.After(3 * time.Second):
				t.Fatal("shutdown ignored its deadline")
			}

			canceled, cancelWait := context.WithCancel(t.Context())
			cancelWait()
			require.ErrorIs(t, engine.Shutdown(canceled), context.Canceled)
			select {
			case <-engine.shutdownDone:
				t.Fatal("shutdown completed before the blocked operation finished")
			default:
			}

			releaseOnce.Do(release)
			waitCtx, cancelWait := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancelWait()
			require.NoError(t, engine.Shutdown(waitCtx))
			require.NoError(t, engine.Shutdown(waitCtx), "completed shutdown can be joined repeatedly")
			_, err = engine.Sync(t.Context())
			require.ErrorContains(t, err, "shut down")
		})
	}
}
