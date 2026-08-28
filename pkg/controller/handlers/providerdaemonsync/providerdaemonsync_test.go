package providerdaemonsync

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingStopper struct {
	authStops  int
	modelStops int
}

func (r *recordingStopper) StopAllAuthProviderDaemons() {
	r.authStops++
}

func (r *recordingStopper) StopAllModelProviderDaemons() {
	r.modelStops++
}

func TestReconcileStopsDaemonsOnlyForNewerGenerations(t *testing.T) {
	stopper := &recordingStopper{}
	handler := &Handler{
		lastGeneration: 3,
		dispatcher:     stopper,
	}

	for _, generation := range []int64{0, 2, 3, 4, 4} {
		require.NoError(t, handler.Reconcile(router.Request{
			Object: &v1.ProviderDaemonSync{
				Spec: v1.ProviderDaemonSyncSpec{
					Generation: generation,
				},
			},
		}, nil))
	}
	assert.Equal(t, 1, stopper.authStops)
	assert.Equal(t, 1, stopper.modelStops)
	assert.Equal(t, int64(4), handler.lastGeneration)
}

func TestReconcileStopsOnFirstObservedGeneration(t *testing.T) {
	stopper := &recordingStopper{}
	handler := New(stopper)

	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderDaemonSync{
			Spec: v1.ProviderDaemonSyncSpec{
				Generation: 9,
			},
		},
	}, nil))
	assert.Equal(t, 1, stopper.authStops)
	assert.Equal(t, 1, stopper.modelStops)

	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderDaemonSync{
			Spec: v1.ProviderDaemonSyncSpec{
				Generation: 9,
			},
		},
	}, nil))
	assert.Equal(t, 1, stopper.authStops)
	assert.Equal(t, 1, stopper.modelStops)
}
