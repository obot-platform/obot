package providerdaemonsync

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestReconcileStopsDaemonsOnlyForNewerTimestamps(t *testing.T) {
	startup := time.Unix(100, 0)
	stopper := &recordingStopper{}
	handler := &Handler{
		lastDaemonRestart: startup,
		dispatcher:        stopper,
	}

	timestamps := []time.Time{
		{},
		startup.Add(-time.Second),
		startup,
		startup.Add(time.Second),
		startup.Add(time.Second),
	}
	for _, timestamp := range timestamps {
		require.NoError(t, handler.Reconcile(router.Request{
			Object: &v1.ProviderDaemonSync{
				Spec: v1.ProviderDaemonSyncSpec{
					Timestamp: metav1.NewTime(timestamp),
				},
			},
		}, nil))
	}
	assert.Equal(t, 1, stopper.authStops)
	assert.Equal(t, 1, stopper.modelStops)
}
