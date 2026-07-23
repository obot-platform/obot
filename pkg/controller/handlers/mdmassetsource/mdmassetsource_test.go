package mdmassetsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncImportsMetadataAndRecordsLatest(t *testing.T) {
	ctx := t.Context()
	fixedTime := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	sourcePath := writeTestMDMAssets(t, "1.2.3")
	gateway := newTestGatewayClient(t)
	source := newTestMDMAssetSource(sourcePath)
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	c := newTestStorageClient(t, source)
	h := New(sourcePath, "https://obot.example", gateway)
	h.now = func() time.Time { return fixedTime }

	resp := runSync(ctx, t, h, c, source)
	assert.Zero(t, resp.Delay, "a healthy source must not be periodically polled")

	var updated v1.MDMAssetSource
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &updated))
	assert.Empty(t, updated.Status.SyncError)
	assert.True(t, fixedTime.Equal(updated.Status.LastSyncTime.Time))
	assert.Len(t, updated.Status.LatestDigest, 64)
	_, refreshRequested := updated.Annotations[v1.MDMAssetSourceSyncAnnotation]
	assert.False(t, refreshRequested)

	var assets v1.MDMAssetList
	require.NoError(t, c.List(ctx, &assets, kclient.InNamespace(system.DefaultNamespace)))
	require.Len(t, assets.Items, 1)
	asset := assets.Items[0]
	assert.Equal(t, updated.Status.LatestDigest, asset.Spec.Digest)
	assert.Equal(t, v1.MDMAssetName(asset.Spec.Digest), asset.Name)
	assert.Equal(t, mdmassets.SchemaVersion, asset.Spec.SchemaVersion)
	assert.Equal(t, "1.2.3", asset.Spec.ObotSentryVersion)
	require.Len(t, asset.Spec.Platforms, 1)
	assert.Equal(t, "intune", asset.Spec.Platforms[0].ID)
	require.Len(t, asset.Spec.Configurations, 1)
	assert.Equal(t, "windows", asset.Spec.Configurations[0].OS)

	bundle, err := gateway.GetMDMAssetBundle(ctx, asset.Spec.Digest)
	require.NoError(t, err)
	loader, err := mdmassets.OpenArchive(bundle.Content)
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", loader.Manifest().ObotSentryVersion)
}

func TestSyncFailurePreservesLatestAndSchedulesRetry(t *testing.T) {
	ctx := t.Context()
	fixedTime := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	missingSource := filepath.Join(t.TempDir(), "missing.tar.gz")
	gateway := newTestGatewayClient(t)
	source := newTestMDMAssetSource(missingSource)
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	source.Status = v1.MDMAssetSourceStatus{
		LastSyncTime: metav1.NewTime(fixedTime.Add(-time.Hour)),
		LatestDigest: "last-known-good",
	}
	c := newTestStorageClient(t, source)
	h := New(missingSource, "https://obot.example", gateway)
	h.now = func() time.Time { return fixedTime }

	resp := runSync(ctx, t, h, c, source)
	assert.Equal(t, retryInterval, resp.Delay)

	var failed v1.MDMAssetSource
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &failed))
	assert.Equal(t, "last-known-good", failed.Status.LatestDigest)
	assert.True(t, fixedTime.Equal(failed.Status.LastSyncTime.Time))
	assert.Contains(t, failed.Status.SyncError, "opening local MDM asset source")
	_, refreshRequested := failed.Annotations[v1.MDMAssetSourceSyncAnnotation]
	assert.False(t, refreshRequested, "the attempted refresh is consumed even when it fails")

	// Controller events during the backoff window must not hammer the source.
	h.now = func() time.Time { return fixedTime.Add(30 * time.Second) }
	retryResp := runSync(ctx, t, h, c, &failed)
	assert.Equal(t, retryInterval-30*time.Second, retryResp.Delay)

	var throttled v1.MDMAssetSource
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &throttled))
	assert.True(t, fixedTime.Equal(throttled.Status.LastSyncTime.Time))
	assert.Equal(t, "last-known-good", throttled.Status.LatestDigest)
	assert.Equal(t, failed.Status.SyncError, throttled.Status.SyncError)
}

func TestSyncEmptySourceClearsStatusWithoutRecordingRefresh(t *testing.T) {
	ctx := t.Context()
	source := newTestMDMAssetSource("")
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	source.Status = v1.MDMAssetSourceStatus{
		LastSyncTime: metav1.Now(),
		SyncError:    "previous failure",
		LatestDigest: "previous-latest",
	}
	c := newTestStorageClient(t, source)
	h := New("", "https://obot.example", newTestGatewayClient(t))

	runSync(ctx, t, h, c, source)

	var updated v1.MDMAssetSource
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &updated))
	assert.True(t, updated.Status.LastSyncTime.IsZero())
	assert.Empty(t, updated.Status.SyncError)
	assert.Empty(t, updated.Status.LatestDigest)
	_, refreshRequested := updated.Annotations[v1.MDMAssetSourceSyncAnnotation]
	assert.False(t, refreshRequested)

	resourceVersion := updated.ResourceVersion
	runSync(ctx, t, h, c, &updated)
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &updated))
	assert.Equal(t, resourceVersion, updated.ResourceVersion, "an already-empty status must not cause another reconciliation")
}

func TestSyncPrunesUnreferencedAssetsAfterSync(t *testing.T) {
	ctx := t.Context()
	gateway := newTestGatewayClient(t)
	oldDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("old"))
	require.NoError(t, err)
	sourcePath := writeTestMDMAssets(t, "2.0.0")
	source := newTestMDMAssetSource(sourcePath)
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	source.Status.LatestDigest = oldDigest
	c := newTestStorageClient(t, source, newTestMDMAsset(oldDigest))
	h := New(sourcePath, "https://obot.example", gateway)

	// The refresh records a new latest, but the outgoing latest is retained
	// while the persisted status still names it.
	runSync(ctx, t, h, c, source)
	var updated v1.MDMAssetSource
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &updated))
	assert.NotEqual(t, oldDigest, updated.Status.LatestDigest)
	var oldAsset v1.MDMAsset
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, v1.MDMAssetName(oldDigest)), &oldAsset))
	_, err = gateway.GetMDMAssetBundle(ctx, oldDigest)
	require.NoError(t, err)

	// The reconciliation that follows the status update collects it.
	runSync(ctx, t, h, c, &updated)
	err = c.Get(ctx, router.Key(source.Namespace, v1.MDMAssetName(oldDigest)), &oldAsset)
	assert.True(t, apierrors.IsNotFound(err), "old metadata error = %v", err)
	_, err = gateway.GetMDMAssetBundle(ctx, oldDigest)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "old bundle error = %v", err)

	// Runtime refreshes prune too: an orphan does not wait for a restart.
	orphanDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("runtime orphan"))
	require.NoError(t, err)
	require.NoError(t, c.Create(ctx, newTestMDMAsset(orphanDigest)))
	require.NoError(t, c.Get(ctx, router.Key(source.Namespace, source.Name), &updated))
	updated.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	require.NoError(t, c.Update(ctx, &updated))
	runSync(ctx, t, h, c, &updated)
	var orphan v1.MDMAsset
	err = c.Get(ctx, router.Key(source.Namespace, v1.MDMAssetName(orphanDigest)), &orphan)
	assert.True(t, apierrors.IsNotFound(err), "orphan metadata error = %v", err)
	_, err = gateway.GetMDMAssetBundle(ctx, orphanDigest)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "orphan bundle error = %v", err)
}

func TestSyncReRendersConfigurationsWhenLatestChanges(t *testing.T) {
	ctx := t.Context()
	gateway := newTestGatewayClient(t)
	oldDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("old source"))
	require.NoError(t, err)
	configured, err := gateway.CreateMDMConfiguration(ctx, 1, &gatewaytypes.MDMConfiguration{
		AssetDigest: oldDigest,
		Values:      `{"interval":60}`,
		Artifacts: []gatewaytypes.MDMConfigurationArtifact{{
			Slug:         "intune-windows",
			Platform:     "intune",
			OS:           "windows",
			Instructions: "Install it",
			Content:      []byte("rendered zip"),
		}},
	})
	require.NoError(t, err)
	blank, err := gateway.CreateMDMConfiguration(ctx, 1, &gatewaytypes.MDMConfiguration{})
	require.NoError(t, err)

	sourcePath := writeTestMDMAssets(t, "2.0.0")
	source := newTestMDMAssetSource(sourcePath)
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	source.Status.LatestDigest = oldDigest
	c := newTestStorageClient(t, source, newTestMDMAsset(oldDigest))
	h := New(sourcePath, "https://obot.example", gateway)

	runSync(ctx, t, h, c, source)
	require.NotEqual(t, oldDigest, source.Status.LatestDigest)

	// Both the stale configuration (its values still validate) and the blank
	// configuration (defaults suffice) render automatically; nobody has to
	// save explicitly.
	storedConfigured, err := gateway.GetMDMConfiguration(ctx, configured.ID)
	require.NoError(t, err)
	assert.Equal(t, `{"interval":60}`, storedConfigured.Values)
	storedBlank, err := gateway.GetMDMConfiguration(ctx, blank.ID)
	require.NoError(t, err)
	assert.Equal(t, `{}`, storedBlank.Values)
	for _, stored := range []*gatewaytypes.MDMConfiguration{storedConfigured, storedBlank} {
		assert.Equal(t, source.Status.LatestDigest, stored.AssetDigest)
		assert.Equal(t, "2.0.0", stored.ObotSentryVersion)
		require.Len(t, stored.Artifacts, 1)
		assert.Equal(t, "intune-windows", stored.Artifacts[0].Slug)
		assert.NotEmpty(t, stored.Artifacts[0].Content)
	}
}

func TestSyncLeavesInvalidConfigurationsForReview(t *testing.T) {
	ctx := t.Context()
	gateway := newTestGatewayClient(t)
	oldDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("old source"))
	require.NoError(t, err)
	configuration, err := gateway.CreateMDMConfiguration(ctx, 1, &gatewaytypes.MDMConfiguration{
		AssetDigest: oldDigest,
		Values:      `{"interval":60}`,
		Artifacts: []gatewaytypes.MDMConfigurationArtifact{{
			Slug:         "intune-windows",
			Platform:     "intune",
			OS:           "windows",
			Instructions: "Install it",
			Content:      []byte("rendered zip"),
		}},
	})
	require.NoError(t, err)

	// The new release requires a value with no default, so the stored values
	// no longer validate and the configuration must wait for explicit review.
	sourcePath := writeTestMDMAssetsWithFields(t, "2.0.0",
		`{"type":"object","required":["team"],"properties":{"serverURL":{"type":"string"},"team":{"type":"string"}}}`)
	source := newTestMDMAssetSource(sourcePath)
	source.Annotations = map[string]string{v1.MDMAssetSourceSyncAnnotation: "true"}
	source.Status.LatestDigest = oldDigest
	c := newTestStorageClient(t, source, newTestMDMAsset(oldDigest))
	h := New(sourcePath, "https://obot.example", gateway)

	runSync(ctx, t, h, c, source)
	stored, err := gateway.GetMDMConfiguration(ctx, configuration.ID)
	require.NoError(t, err)
	assert.Equal(t, oldDigest, stored.AssetDigest)
	assert.Equal(t, `{"interval":60}`, stored.Values)
	assert.Empty(t, stored.Artifacts)
}

func TestPruneUnusedRetainsLatestAndConfigurationPins(t *testing.T) {
	ctx := t.Context()
	gateway := newTestGatewayClient(t)
	latestDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("latest"))
	require.NoError(t, err)
	pinnedDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("pinned"))
	require.NoError(t, err)
	orphanDigest, err := gateway.StoreMDMAssetBundle(ctx, []byte("orphan"))
	require.NoError(t, err)

	_, err = gateway.CreateMDMConfiguration(ctx, 1, &gatewaytypes.MDMConfiguration{
		AssetDigest: pinnedDigest,
		Values:      `{}`,
		Artifacts: []gatewaytypes.MDMConfigurationArtifact{{
			Slug:         "intune-windows",
			Platform:     "intune",
			OS:           "windows",
			Instructions: "Install it",
			Content:      []byte("rendered zip"),
		}},
	})
	require.NoError(t, err)

	source := newTestMDMAssetSource("/srv/mdm/assets")
	source.Status.LatestDigest = latestDigest
	latest := newTestMDMAsset(latestDigest)
	pinned := newTestMDMAsset(pinnedDigest)
	orphan := newTestMDMAsset(orphanDigest)
	c := newTestStorageClient(t, source, latest, pinned, orphan)
	h := New(source.Spec.Source, "https://obot.example", gateway)

	require.NoError(t, h.pruneUnused(ctx, c))

	for _, asset := range []*v1.MDMAsset{latest, pinned} {
		var stored v1.MDMAsset
		require.NoError(t, c.Get(ctx, router.Key(asset.Namespace, asset.Name), &stored))
		_, err := gateway.GetMDMAssetBundle(ctx, asset.Spec.Digest)
		require.NoError(t, err)
	}
	var deleted v1.MDMAsset
	err = c.Get(ctx, router.Key(orphan.Namespace, orphan.Name), &deleted)
	assert.True(t, apierrors.IsNotFound(err), "orphan metadata error = %v", err)
	_, err = gateway.GetMDMAssetBundle(ctx, orphanDigest)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound), "orphan bundle error = %v", err)
}

func newTestStorageClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MDMAssetSource{}).
		WithObjects(objects...).
		Build()
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })
	return gateway
}

func newTestMDMAssetSource(source string) *v1.MDMAssetSource {
	return &v1.MDMAssetSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MDMAssetSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultMDMAssetSource,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSourceSpec{
			MDMAssetSourceManifest: clienttypes.MDMAssetSourceManifest{Source: source},
		},
	}
}

func newTestMDMAsset(digest string) *v1.MDMAsset {
	return &v1.MDMAsset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1.MDMAssetName(digest),
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSpec{Digest: digest},
	}
}

// runSync invokes Sync the way the router does: the handler mutates
// req.Object, and any status change is persisted after it returns.
func runSync(ctx context.Context, t *testing.T, h *Handler, c kclient.WithWatch, source *v1.MDMAssetSource) *router.ResponseWrapper {
	t.Helper()
	resp := &router.ResponseWrapper{}
	unmodified := source.DeepCopy()
	require.NoError(t, h.Sync(newTestRequest(ctx, c, source), resp))
	if router.StatusChanged(unmodified, source) {
		require.NoError(t, c.Status().Update(ctx, source))
	}
	return resp
}

func newTestRequest(ctx context.Context, c kclient.WithWatch, source *v1.MDMAssetSource) router.Request {
	return router.Request{
		Client:    c,
		Object:    source,
		Ctx:       ctx,
		Namespace: source.Namespace,
		Name:      source.Name,
		Key:       source.Namespace + "/" + source.Name,
	}
}

func writeTestMDMAssets(t *testing.T, version string) string {
	t.Helper()
	return writeTestMDMAssetsWithFields(t, version, `{"type":"object","properties":{"serverURL":{"type":"string"}}}`)
}

func writeTestMDMAssetsWithFields(t *testing.T, version, fields string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := fmt.Sprintf(`{
  "schemaVersion":"v1",
  "obotSentryVersion":%q,
  "fields":%s,
  "platforms":[{"id":"intune","label":"Intune"}],
  "configurations":[{"platform":"intune","os":"windows","osLabel":"Windows","instructions":"instructions.md.tmpl","assets":["package.bin","instructions.md.tmpl"]}]
}`, version, fields)
	for name, content := range map[string]string{
		"manifest.json":        manifest,
		"package.bin":          "package",
		"instructions.md.tmpl": "server={{.serverURL}}",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	return dir
}
