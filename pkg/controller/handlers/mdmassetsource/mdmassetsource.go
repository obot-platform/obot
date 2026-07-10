package mdmassetsource

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const retryInterval = time.Minute

type Handler struct {
	defaultSource string
	gatewayClient *gatewayclient.Client
	now           func() time.Time
	pruneLock     sync.Mutex
	pruned        bool
}

func New(defaultSource string, gatewayClient *gatewayclient.Client) *Handler {
	return &Handler{
		defaultSource: strings.TrimSpace(defaultSource),
		gatewayClient: gatewayClient,
		now:           time.Now,
	}
}

// Sync imports a source on creation, explicit refresh, startup source change,
// or retry after failure. Healthy sources are not polled.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	source := req.Object.(*v1.MDMAssetSource)
	if source.Namespace != system.DefaultNamespace || source.Name != system.DefaultMDMAssetSource {
		return nil
	}
	if source.Spec.Source != h.defaultSource {
		source.Spec.Source = h.defaultSource
		if source.Annotations == nil {
			source.Annotations = map[string]string{}
		}
		source.Annotations[v1.MDMAssetSourceSyncAnnotation] = "true"
		return req.Client.Update(req.Ctx, source)
	}

	refreshRequested := source.Annotations[v1.MDMAssetSourceSyncAnnotation] == "true"
	if !refreshRequested && !source.Status.LastSyncTime.IsZero() && source.Status.SyncError != "" {
		if elapsed := h.now().Sub(source.Status.LastSyncTime.Time); elapsed < retryInterval {
			if err := h.pruneOnce(req.Ctx, req.Client); err != nil {
				return err
			}
			resp.RetryAfter(retryInterval - elapsed)
			return nil
		}
	}
	if !refreshRequested && !source.Status.LastSyncTime.IsZero() && source.Status.SyncError == "" {
		return h.pruneOnce(req.Ctx, req.Client)
	}

	digest, err := h.sync(req.Ctx, req.Client, source)
	if err != nil {
		message := sanitizedError(err, source.Spec.Source)
		if statusErr := h.recordFailure(req.Ctx, req.Client, source.Namespace, source.Name, message); statusErr != nil {
			return fmt.Errorf("%s; failed to record MDM asset source error: %w", message, statusErr)
		}
		if refreshRequested {
			if err := clearSyncAnnotation(req.Ctx, req.Client, source.Namespace, source.Name); err != nil {
				return err
			}
		}
		log.Errorf("Failed to sync MDM asset source %s: %s", source.Name, message)
		if err := h.pruneOnce(req.Ctx, req.Client); err != nil {
			return err
		}
		resp.RetryAfter(retryInterval)
		return nil
	}

	if err := h.recordSuccess(req.Ctx, req.Client, source.Namespace, source.Name, digest); err != nil {
		return err
	}
	if refreshRequested {
		if err := clearSyncAnnotation(req.Ctx, req.Client, source.Namespace, source.Name); err != nil {
			return err
		}
	}
	return h.pruneOnce(req.Ctx, req.Client)
}

func (h *Handler) sync(ctx context.Context, c kclient.Client, source *v1.MDMAssetSource) (string, error) {
	if strings.TrimSpace(source.Spec.Source) == "" {
		return "", nil
	}

	content, err := mdmassets.Import(ctx, source.Spec.Source)
	if err != nil {
		return "", fmt.Errorf("importing MDM assets: %w", err)
	}
	loader, err := mdmassets.OpenArchive(content)
	if err != nil {
		return "", fmt.Errorf("opening imported MDM assets: %w", err)
	}
	digest, err := h.gatewayClient.StoreMDMAssetBundle(ctx, content)
	if err != nil {
		return "", err
	}

	asset := &v1.MDMAsset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1.MDMAssetName(digest),
			Namespace: source.Namespace,
		},
		Spec: v1.MDMAssetSpec{
			Digest:           digest,
			MDMAssetManifest: loader.Manifest(),
		},
	}
	var existing v1.MDMAsset
	if err := c.Get(ctx, router.Key(source.Namespace, asset.Name), &existing); apierrors.IsNotFound(err) {
		if err := c.Create(ctx, asset); apierrors.IsAlreadyExists(err) {
			if err := c.Get(ctx, router.Key(source.Namespace, asset.Name), &existing); err != nil {
				return "", fmt.Errorf("checking concurrently created MDM asset metadata: %w", err)
			}
			if existing.Spec.Digest != digest {
				return "", fmt.Errorf("MDM asset name collision for digest %s", shortDigest(digest))
			}
		} else if err != nil {
			return "", fmt.Errorf("creating MDM asset metadata: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("checking MDM asset metadata: %w", err)
	} else if existing.Spec.Digest != digest {
		return "", fmt.Errorf("MDM asset name collision for digest %s", shortDigest(digest))
	}

	return digest, nil
}

func (h *Handler) recordFailure(ctx context.Context, c kclient.Client, namespace, name, message string) error {
	var source v1.MDMAssetSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload MDM asset source: %w", err)
	}
	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = message
	return c.Status().Update(ctx, &source)
}

func (h *Handler) recordSuccess(ctx context.Context, c kclient.Client, namespace, name, digest string) error {
	var source v1.MDMAssetSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload MDM asset source: %w", err)
	}
	source.Status.LastSyncTime = metav1.NewTime(h.now())
	source.Status.SyncError = ""
	source.Status.LatestDigest = digest
	return c.Status().Update(ctx, &source)
}

func clearSyncAnnotation(ctx context.Context, c kclient.Client, namespace, name string) error {
	var source v1.MDMAssetSource
	if err := c.Get(ctx, router.Key(namespace, name), &source); err != nil {
		return fmt.Errorf("failed to reload MDM asset source for annotation cleanup: %w", err)
	}
	if source.Annotations == nil {
		return nil
	}
	if _, ok := source.Annotations[v1.MDMAssetSourceSyncAnnotation]; !ok {
		return nil
	}
	delete(source.Annotations, v1.MDMAssetSourceSyncAnnotation)
	return c.Update(ctx, &source)
}

func (h *Handler) pruneOnce(ctx context.Context, c kclient.Client) error {
	h.pruneLock.Lock()
	defer h.pruneLock.Unlock()
	if h.pruned {
		return nil
	}
	if err := h.PruneUnused(ctx, c); err != nil {
		return err
	}
	h.pruned = true
	return nil
}

// SetUpDefaultMDMAssetSource creates the singleton and makes startup
// configuration authoritative. A changed source triggers reconciliation.
func (h *Handler) SetUpDefaultMDMAssetSource(ctx context.Context, c kclient.Client) error {
	key := router.Key(system.DefaultNamespace, system.DefaultMDMAssetSource)
	var source v1.MDMAssetSource
	if err := c.Get(ctx, key, &source); apierrors.IsNotFound(err) {
		source = v1.MDMAssetSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.DefaultMDMAssetSource,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MDMAssetSourceSpec{
				MDMAssetSourceManifest: types.MDMAssetSourceManifest{Source: h.defaultSource},
			},
		}
		if err := c.Create(ctx, &source); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return fmt.Errorf("failed to create default MDM asset source: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get default MDM asset source: %w", err)
	}
	if source.Spec.Source == h.defaultSource {
		return nil
	}
	source.Spec.Source = h.defaultSource
	if source.Annotations == nil {
		source.Annotations = map[string]string{}
	}
	source.Annotations[v1.MDMAssetSourceSyncAnnotation] = "true"
	if err := c.Update(ctx, &source); err != nil {
		return fmt.Errorf("failed to update default MDM asset source: %w", err)
	}
	return nil
}

// PruneUnused removes unreferenced asset metadata and private blobs once at
// startup. Runtime refreshes never prune historical pins.
func (h *Handler) PruneUnused(ctx context.Context, c kclient.Client) error {
	var source v1.MDMAssetSource
	err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultMDMAssetSource), &source)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get MDM asset source for pruning: %w", err)
	}
	latestDigest := source.Status.LatestDigest

	configurations, err := h.gatewayClient.ListMDMConfigurations(ctx)
	if err != nil {
		return err
	}
	referenced := make(map[string]struct{}, len(configurations)+1)
	if latestDigest != "" {
		referenced[latestDigest] = struct{}{}
	}
	for _, configuration := range configurations {
		if configuration.AssetDigest != "" {
			referenced[configuration.AssetDigest] = struct{}{}
		}
	}

	var assets v1.MDMAssetList
	if err := c.List(ctx, &assets, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return fmt.Errorf("failed to list MDM assets for pruning: %w", err)
	}
	for i := range assets.Items {
		asset := &assets.Items[i]
		if _, ok := referenced[asset.Spec.Digest]; ok {
			continue
		}
		if err := c.Delete(ctx, asset); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to prune MDM asset %s: %w", shortDigest(asset.Spec.Digest), err)
		}
	}

	return h.gatewayClient.PruneUnusedMDMAssetBundles(ctx, latestDigest)
}

func sanitizedError(err error, source string) string {
	message := err.Error()
	if source != "" {
		message = strings.ReplaceAll(message, source, mdmassets.RedactSource(source))
	}
	const maxErrorLength = 2048
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength]
	}
	return message
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}
