package mdmassetsource

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

// Failed imports retry hourly, matching the SkillRepository controller. A
// successful source is only reconciled again when an admin requests a refresh
// or the startup source changes.
const retryInterval = time.Hour

type Handler struct {
	defaultSource string
	serverURL     string
	gatewayClient *gatewayclient.Client
	now           func() time.Time
}

func New(defaultSource, serverURL string, gatewayClient *gatewayclient.Client) *Handler {
	return &Handler{
		defaultSource: strings.TrimSpace(defaultSource),
		serverURL:     serverURL,
		gatewayClient: gatewayClient,
		now:           time.Now,
	}
}

// Sync imports a source on creation, explicit refresh, startup source change,
// or retry after failure. Healthy sources are not polled. Status changes are
// persisted by the router after the handler returns.
func (h *Handler) Sync(req router.Request, resp router.Response) error {
	source := req.Object.(*v1.MDMAssetSource)
	if source.Spec.Source != h.defaultSource {
		source.Spec.Source = h.defaultSource
		if source.Annotations == nil {
			source.Annotations = map[string]string{}
		}
		source.Annotations[v1.MDMAssetSourceSyncAnnotation] = "true"
		return req.Client.Update(req.Ctx, source)
	}

	refreshRequested := source.Annotations[v1.MDMAssetSourceSyncAnnotation] == "true"
	if !refreshRequested && !source.Status.LastSyncTime.IsZero() {
		if source.Status.SyncError == "" {
			return h.pruneUnused(req.Ctx, req.Client)
		}
		if elapsed := h.now().Sub(source.Status.LastSyncTime.Time); elapsed < retryInterval {
			resp.RetryAfter(retryInterval - elapsed)
			return nil
		}
	}

	previousDigest := source.Status.LatestDigest
	digest, err := h.sync(req.Ctx, req.Client, source)
	if refreshRequested {
		// The refresh is consumed whether or not it succeeded. This update also
		// refreshes source in place, so the router's trailing status update
		// writes against the current resource version.
		delete(source.Annotations, v1.MDMAssetSourceSyncAnnotation)
		if err := req.Client.Update(req.Ctx, source); err != nil {
			return err
		}
	}
	if err != nil {
		message := sanitizedError(err, source.Spec.Source)
		log.Errorf("Failed to sync MDM asset source %s: %s", source.Name, message)
		source.Status.LastSyncTime = metav1.NewTime(h.now())
		source.Status.SyncError = message
		resp.RetryAfter(retryInterval)
		return nil
	}
	if digest != previousDigest {
		if err := h.gatewayClient.InvalidateMDMConfigurationArtifacts(req.Ctx, digest); err != nil {
			return err
		}
		if digest != "" {
			if err := h.renderConfigurationsForLatest(req.Ctx, digest); err != nil {
				return err
			}
		}
	}

	if digest == "" {
		source.Status = v1.MDMAssetSourceStatus{}
	} else {
		source.Status.LastSyncTime = metav1.NewTime(h.now())
		source.Status.SyncError = ""
		source.Status.LatestDigest = digest
	}
	return h.pruneUnused(req.Ctx, req.Client, digest)
}

// renderConfigurationsForLatest re-renders every configuration that is not
// already on the latest bundle so administrators only save explicitly when
// input is genuinely required. Stored values (or defaults, for blank
// configurations) are validated against the new fields first; configurations
// that no longer validate keep their invalidated state for explicit review.
func (h *Handler) renderConfigurationsForLatest(ctx context.Context, digest string) error {
	configurations, err := h.gatewayClient.ListMDMConfigurations(ctx)
	if err != nil {
		return err
	}

	var loader *mdmassets.Loader
	for _, configuration := range configurations {
		if configuration.AssetDigest == digest {
			continue
		}
		if loader == nil {
			bundle, err := h.gatewayClient.GetMDMAssetBundle(ctx, digest)
			if err != nil {
				return err
			}
			if loader, err = mdmassets.OpenArchive(bundle.Content); err != nil {
				return err
			}
		}
		rendered, err := h.renderConfiguration(loader, digest, configuration)
		if err != nil {
			log.Infof("MDM configuration %d requires review before rendering against the latest release: %v", configuration.ID, err)
			continue
		}
		if err := h.gatewayClient.UpdateMDMConfiguration(ctx, &rendered); err != nil {
			return err
		}
	}
	return nil
}

// renderConfiguration validates the configuration's stored values against the
// bundle and renders every target, mirroring the values handling of the save
// API: the caller-visible values stay sparse and the trusted server URL is
// injected only for rendering.
func (h *Handler) renderConfiguration(loader *mdmassets.Loader, digest string, configuration gatewaytypes.MDMConfiguration) (gatewaytypes.MDMConfiguration, error) {
	values := map[string]any{}
	if configuration.Values != "" {
		if err := json.Unmarshal([]byte(configuration.Values), &values); err != nil {
			return gatewaytypes.MDMConfiguration{}, fmt.Errorf("stored values are not valid JSON: %w", err)
		}
	}
	delete(values, "serverURL")
	renderValues := make(map[string]any, len(values)+1)
	maps.Copy(renderValues, values)
	renderValues["serverURL"] = h.serverURL
	rendered, err := loader.RenderAll(renderValues)
	if err != nil {
		return gatewaytypes.MDMConfiguration{}, err
	}
	if len(rendered) == 0 {
		return gatewaytypes.MDMConfiguration{}, fmt.Errorf("the bundle has no platform/OS configurations")
	}

	storedValues, err := json.Marshal(values)
	if err != nil {
		return gatewaytypes.MDMConfiguration{}, err
	}
	configuration.AssetDigest = digest
	configuration.ObotSentryVersion = loader.Manifest().ObotSentryVersion
	configuration.Values = string(storedValues)
	configuration.Artifacts = make([]gatewaytypes.MDMConfigurationArtifact, 0, len(rendered))
	for _, artifact := range rendered {
		configuration.Artifacts = append(configuration.Artifacts, gatewaytypes.MDMConfigurationArtifact{
			Slug:         mdmassets.ArtifactSlug(artifact.Platform, artifact.OS),
			Platform:     artifact.Platform,
			OS:           artifact.OS,
			Instructions: artifact.Instructions,
			Content:      artifact.Content,
		})
	}
	return configuration, nil
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

	manifest := loader.Manifest()
	asset := &v1.MDMAsset{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v1.MDMAssetName(digest),
			Namespace: source.Namespace,
		},
		Spec: v1.MDMAssetSpec{
			Digest:            digest,
			SchemaVersion:     manifest.SchemaVersion,
			ObotSentryVersion: manifest.ObotSentryVersion,
			Fields:            runtime.RawExtension{Raw: manifest.Fields},
			Platforms:         manifest.Platforms,
			Configurations:    manifest.Configurations,
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

// pruneUnused removes asset metadata and private source bundles that are neither
// the latest digest, pinned by an MDM configuration, nor explicitly retained
// by the caller. The persisted latest is read fresh and retained alongside any
// caller-supplied digest: after a sync, the new latest is not saved until the
// handler returns, and the previous one may still win if that save fails, so
// neither may be collected yet. The reconciliation triggered by the save
// collects the loser once nothing pins it.
func (h *Handler) pruneUnused(ctx context.Context, c kclient.Client, retainDigests ...string) error {
	var source v1.MDMAssetSource
	err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultMDMAssetSource), &source)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get MDM asset source for pruning: %w", err)
	}
	retained := make([]string, 0, len(retainDigests)+1)
	for _, digest := range append(retainDigests, source.Status.LatestDigest) {
		if digest != "" && !slices.Contains(retained, digest) {
			retained = append(retained, digest)
		}
	}

	configurations, err := h.gatewayClient.ListMDMConfigurations(ctx)
	if err != nil {
		return err
	}
	referenced := make(map[string]struct{}, len(configurations)+len(retained))
	for _, digest := range retained {
		referenced[digest] = struct{}{}
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

	retained = retained[:0]
	for digest := range referenced {
		retained = append(retained, digest)
	}
	return h.gatewayClient.PruneUnusedMDMAssetBundles(ctx, retained...)
}

// SetUpDefaultMDMAssetSource creates the default MDMAssetSource at startup.
// Changes to the default source value are picked up by `Handler.Sync` during normal reconciliation.
func (h *Handler) SetUpDefaultMDMAssetSource(ctx context.Context, c kclient.Client) error {
	source := v1.MDMAssetSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultMDMAssetSource,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MDMAssetSourceSpec{
			MDMAssetSourceManifest: types.MDMAssetSourceManifest{Source: h.defaultSource},
		},
	}

	if err := kclient.IgnoreAlreadyExists(c.Create(ctx, &source)); err != nil {
		return fmt.Errorf("failed to create default MDM asset source: %w", err)
	}

	return nil
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
