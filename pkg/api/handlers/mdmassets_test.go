package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMDMAssetSourceHandlerGetRedactsSourceAndReportsPendingRefresh(t *testing.T) {
	lastSyncTime := metav1.NewTime(time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC))
	storage := newFakeStorage(t, &v1.MDMAssetSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultMDMAssetSource,
			Namespace: system.DefaultNamespace,
			Annotations: map[string]string{
				v1.MDMAssetSourceSyncAnnotation: "true",
			},
		},
		Spec: v1.MDMAssetSourceSpec{
			MDMAssetSourceManifest: apitypes.MDMAssetSourceManifest{
				Source: "https://user:secret@example.com/releases/mdm.tar.gz?token=secret#fragment",
			},
		},
		Status: v1.MDMAssetSourceStatus{
			LastSyncTime: lastSyncTime,
			SyncError:    "last refresh failed",
			LatestDigest: "sha256:latest",
		},
	})
	recorder := httptest.NewRecorder()

	err := NewMDMAssetSourceHandler().Get(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/mdm/asset-source", nil),
		Storage:        storage,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var got apitypes.MDMAssetSource
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &got))
	assert.Equal(t, system.DefaultMDMAssetSource, got.ID)
	assert.Equal(t, "https://example.com/releases/mdm.tar.gz", got.Source)
	assert.NotContains(t, recorder.Body.String(), "secret")
	assert.True(t, got.IsSyncing, "a pending refresh annotation must be visible before reconciliation starts")
	assert.Equal(t, "last refresh failed", got.SyncError)
	assert.Equal(t, "sha256:latest", got.LatestDigest)
	assert.True(t, lastSyncTime.Time.Equal(got.LastSyncTime.Time))
}

func TestMDMAssetSourceHandlerRefreshOnlyRequestsReconciliation(t *testing.T) {
	const configuredSource = "/etc/obot/mdm-assets.tar.gz"
	storage := newFakeStorage(t, &v1.MDMAssetSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultMDMAssetSource,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSourceSpec{
			MDMAssetSourceManifest: apitypes.MDMAssetSourceManifest{Source: configuredSource},
		},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/mdm/asset-source/refresh",
		// A caller cannot use refresh as an update endpoint.
		strings.NewReader(`{"source":"https://attacker.invalid/replacement.tar.gz"}`),
	)

	err := NewMDMAssetSourceHandler().Refresh(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, recorder.Code)

	var source v1.MDMAssetSource
	require.NoError(t, storage.Get(context.Background(), kclient.ObjectKey{
		Name:      system.DefaultMDMAssetSource,
		Namespace: system.DefaultNamespace,
	}, &source))
	assert.Equal(t, configuredSource, source.Spec.Source)
	assert.Equal(t, "true", source.Annotations[v1.MDMAssetSourceSyncAnnotation])
}

func TestMDMAssetHandlerListExposesManifestMetadata(t *testing.T) {
	const digest = "sha256:0123456789abcdef"
	manifest := apitypes.MDMAssetManifest{
		SchemaVersion:     "v1",
		ObotSentryVersion: "1.2.3",
		Fields:            json.RawMessage(`{"serverURL":{"type":"string"}}`),
		Platforms: []apitypes.MDMAssetPlatform{
			{ID: "apple", Label: "Apple", Icon: "apple"},
		},
		Configurations: []apitypes.MDMAssetConfiguration{
			{
				Platform:      "apple",
				OS:            "macos",
				OSLabel:       "macOS",
				Description:   "Enroll a Mac",
				SuggestedName: "Managed Mac",
				Instructions:  "Follow the prompts",
				Assets:        []string{"profile.mobileconfig.tmpl"},
			},
		},
	}
	storage := newFakeStorage(t, &v1.MDMAsset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1.MDMAssetName(digest),
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSpec{
			Digest:           digest,
			MDMAssetManifest: manifest,
		},
	})
	handler := NewMDMAssetHandler()

	listRecorder := httptest.NewRecorder()
	err := handler.List(api.Context{
		ResponseWriter: listRecorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/mdm/assets", nil),
		Storage:        storage,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, listRecorder.Code)

	var list apitypes.MDMAssetList
	require.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, digest, list.Items[0].Digest)
	assert.Equal(t, manifest.SchemaVersion, list.Items[0].SchemaVersion)
	assert.Equal(t, manifest.ObotSentryVersion, list.Items[0].ObotSentryVersion)
	assert.JSONEq(t, string(manifest.Fields), string(list.Items[0].Fields))
	assert.Equal(t, manifest.Platforms, list.Items[0].Platforms)
	assert.Equal(t, manifest.Configurations, list.Items[0].Configurations)
}
