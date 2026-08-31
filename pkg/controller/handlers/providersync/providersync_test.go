package providersync

import (
	"context"
	"errors"
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

type recordingLicenseRefresher struct {
	refreshes int
	err       error
}

func (r *recordingLicenseRefresher) Validate(context.Context) error {
	r.refreshes++
	return r.err
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
	handler := New(stopper, &recordingLicenseRefresher{})
	daemonSync := &v1.ProviderSync{
		UID: types.UID("sync-1"),
		Spec: v1.ProviderSyncSpec{
			Revisions: map[string]v1.ProviderRevision{
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
	daemonSync.Spec.Revisions["model/default/anthropic"] = v1.ProviderRevision{
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
	handler := New(stopper, &recordingLicenseRefresher{})
	revision := v1.ProviderRevision{
		ProviderType:      v1.ProviderTypeAuth,
		ProviderNamespace: "default",
		ProviderName:      "entra",
		Revision:          9,
	}

	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderSync{
			UID: types.UID("sync-1"),
			Spec: v1.ProviderSyncSpec{
				Revisions: map[string]v1.ProviderRevision{
					"auth/default/entra": revision,
				},
			},
		},
	}, nil))

	revision.Revision = 1
	require.NoError(t, handler.Reconcile(router.Request{
		Object: &v1.ProviderSync{
			UID: types.UID("sync-2"),
			Spec: v1.ProviderSyncSpec{
				Revisions: map[string]v1.ProviderRevision{
					"auth/default/entra": revision,
				},
			},
		},
	}, nil))
	assert.Equal(t, 2, stopper.authStops["default/entra"])
}

func TestReconcileRejectsInvalidProviderType(t *testing.T) {
	stopper := newRecordingStopper()
	handler := New(stopper, &recordingLicenseRefresher{})

	err := handler.Reconcile(router.Request{
		Object: &v1.ProviderSync{
			Spec: v1.ProviderSyncSpec{
				Revisions: map[string]v1.ProviderRevision{
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

func TestReconcileRefreshesLicenseForEachNewRevision(t *testing.T) {
	refresherOne := &recordingLicenseRefresher{}
	refresherTwo := &recordingLicenseRefresher{}
	handlerOne := New(newRecordingStopper(), refresherOne)
	handlerTwo := New(newRecordingStopper(), refresherTwo)
	daemonSync := &v1.ProviderSync{
		UID: types.UID("sync-1"),
		Spec: v1.ProviderSyncSpec{
			Revisions: map[string]v1.ProviderRevision{
				string(v1.ProviderTypeLicense): {
					ProviderType: v1.ProviderTypeLicense,
					Revision:     1,
				},
			},
		},
	}

	require.NoError(t, handlerOne.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	require.NoError(t, handlerTwo.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	assert.Equal(t, 1, refresherOne.refreshes)
	assert.Equal(t, 1, refresherTwo.refreshes)

	require.NoError(t, handlerOne.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	require.NoError(t, handlerTwo.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	assert.Equal(t, 1, refresherOne.refreshes)
	assert.Equal(t, 1, refresherTwo.refreshes)

	licenseRevision := daemonSync.Spec.Revisions[string(v1.ProviderTypeLicense)]
	licenseRevision.Revision++
	daemonSync.Spec.Revisions[string(v1.ProviderTypeLicense)] = licenseRevision
	require.NoError(t, handlerOne.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	require.NoError(t, handlerTwo.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	assert.Equal(t, 2, refresherOne.refreshes)
	assert.Equal(t, 2, refresherTwo.refreshes)

	daemonSync.UID = types.UID("sync-2")
	licenseRevision.Revision = 1
	daemonSync.Spec.Revisions[string(v1.ProviderTypeLicense)] = licenseRevision
	require.NoError(t, handlerOne.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	require.NoError(t, handlerTwo.Reconcile(router.Request{Object: daemonSync, Ctx: t.Context()}, nil))
	assert.Equal(t, 3, refresherOne.refreshes)
	assert.Equal(t, 3, refresherTwo.refreshes)
}

func TestReconcileRetriesFailedLicenseRefresh(t *testing.T) {
	refresher := &recordingLicenseRefresher{err: errors.New("refresh failed")}
	handler := New(newRecordingStopper(), refresher)
	req := router.Request{
		Object: &v1.ProviderSync{
			UID: types.UID("sync-1"),
			Spec: v1.ProviderSyncSpec{
				Revisions: map[string]v1.ProviderRevision{
					string(v1.ProviderTypeLicense): {
						ProviderType: v1.ProviderTypeLicense,
						Revision:     1,
					},
				},
			},
		},
		Ctx: t.Context(),
	}

	require.ErrorContains(t, handler.Reconcile(req, nil), "refresh failed")
	refresher.err = nil
	require.NoError(t, handler.Reconcile(req, nil))
	assert.Equal(t, 2, refresher.refreshes)
}
