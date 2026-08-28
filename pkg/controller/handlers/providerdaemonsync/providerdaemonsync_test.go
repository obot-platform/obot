package providerdaemonsync

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

type recordingStopper struct {
	authStops  map[string]int
	modelStops map[string]int
}

func newRecordingStopper() *recordingStopper {
	return &recordingStopper{
		authStops:  make(map[string]int),
		modelStops: make(map[string]int),
	}
}

func (r *recordingStopper) StopAuthProvider(namespace, authProviderName string) {
	r.authStops[namespace+"/"+authProviderName]++
}

func (r *recordingStopper) StopModelProvider(namespace, modelProviderName string) {
	r.modelStops[namespace+"/"+modelProviderName]++
}

func TestReconcileStopsOnlyProvidersWithNewRevisions(t *testing.T) {
	stopper := newRecordingStopper()
	handler := New(stopper)
	daemonSync := &v1.ProviderDaemonSync{
		UID: types.UID("sync-1"),
		Spec: v1.ProviderDaemonSyncSpec{
			Revisions: map[string]v1.ProviderDaemonRevision{
				"auth/default/entra": {
					ProviderType:      v1.ProviderTypeAuth,
					ProviderNamespace: "default",
					ProviderName:      "entra",
					Revision:          2,
				},
				"model/default/openai": {
					ProviderType:      v1.ProviderTypeModel,
					ProviderNamespace: "default",
					ProviderName:      "openai",
					Revision:          4,
				},
			},
		},
	}

	require.NoError(t, handler.Reconcile(router.Request{Object: daemonSync}, nil))
	assert.Equal(t, 1, stopper.authStops["default/entra"])
	assert.Equal(t, 1, stopper.modelStops["default/openai"])

	require.NoError(t, handler.Reconcile(router.Request{Object: daemonSync}, nil))
	assert.Equal(t, 1, stopper.authStops["default/entra"])
	assert.Equal(t, 1, stopper.modelStops["default/openai"])

	authRevision := daemonSync.Spec.Revisions["auth/default/entra"]
	authRevision.Revision++
	daemonSync.Spec.Revisions["auth/default/entra"] = authRevision
	daemonSync.Spec.Revisions["model/default/anthropic"] = v1.ProviderDaemonRevision{
		ProviderType:      v1.ProviderTypeModel,
		ProviderNamespace: "default",
		ProviderName:      "anthropic",
		Revision:          3,
	}
	require.NoError(t, handler.Reconcile(router.Request{Object: daemonSync}, nil))
	assert.Equal(t, 2, stopper.authStops["default/entra"])
	assert.Equal(t, 1, stopper.modelStops["default/openai"])
	assert.Equal(t, 1, stopper.modelStops["default/anthropic"])

	authRevision.Revision = 1
	daemonSync.Spec.Revisions["auth/default/entra"] = authRevision
	require.NoError(t, handler.Reconcile(router.Request{Object: daemonSync}, nil))
	assert.Equal(t, 2, stopper.authStops["default/entra"])
}

func TestReconcileTreatsRecreatedSyncAsNew(t *testing.T) {
	stopper := newRecordingStopper()
	handler := New(stopper)
	revision := v1.ProviderDaemonRevision{
		ProviderType:      v1.ProviderTypeAuth,
		ProviderNamespace: "default",
		ProviderName:      "entra",
		Revision:          9,
	}

	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderDaemonSync{
			UID: types.UID("sync-1"),
			Spec: v1.ProviderDaemonSyncSpec{
				Revisions: map[string]v1.ProviderDaemonRevision{
					"auth/default/entra": revision,
				},
			},
		},
	}, nil))

	revision.Revision = 1
	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderDaemonSync{
			UID: types.UID("sync-2"),
			Spec: v1.ProviderDaemonSyncSpec{
				Revisions: map[string]v1.ProviderDaemonRevision{
					"auth/default/entra": revision,
				},
			},
		},
	}, nil))
	assert.Equal(t, 2, stopper.authStops["default/entra"])
}

func TestReconcileRejectsInvalidProviderType(t *testing.T) {
	stopper := newRecordingStopper()
	handler := New(stopper)

	err := handler.Reconcile(router.Request{
		Object: &v1.ProviderDaemonSync{
			Spec: v1.ProviderDaemonSyncSpec{
				Revisions: map[string]v1.ProviderDaemonRevision{
					"invalid": {
						ProviderType:      "invalid",
						ProviderNamespace: "default",
						ProviderName:      "provider",
						Revision:          1,
					},
				},
			},
		},
	}, nil)
	require.ErrorContains(t, err, "invalid provider type")
	assert.Empty(t, stopper.authStops)
	assert.Empty(t, stopper.modelStops)
}
