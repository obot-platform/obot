package handlers

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mdmassets"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type MDMAssetSourceHandler struct{}

func NewMDMAssetSourceHandler() *MDMAssetSourceHandler { return nil }

func (*MDMAssetSourceHandler) Get(req api.Context) error {
	source, err := getMDMAssetSource(req)
	if err != nil {
		return err
	}
	return req.Write(convertMDMAssetSource(*source))
}

func (*MDMAssetSourceHandler) Refresh(req api.Context) error {
	source, err := getMDMAssetSource(req)
	if err != nil {
		return err
	}
	if source.Annotations == nil {
		source.Annotations = map[string]string{}
	}
	source.Annotations[v1.MDMAssetSourceSyncAnnotation] = "true"
	if err := req.Update(source); err != nil {
		return fmt.Errorf("failed to refresh MDM asset source: %w", err)
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}

type MDMAssetHandler struct{}

func NewMDMAssetHandler() *MDMAssetHandler { return nil }

func (*MDMAssetHandler) List(req api.Context) error {
	var list v1.MDMAssetList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list MDM assets: %w", err)
	}
	items := make([]types.MDMAsset, 0, len(list.Items))
	for _, asset := range list.Items {
		items = append(items, convertMDMAsset(asset))
	}
	return req.Write(types.MDMAssetList{Items: items})
}

func getMDMAssetSource(req api.Context) (*v1.MDMAssetSource, error) {
	var source v1.MDMAssetSource
	if err := req.Get(&source, system.DefaultMDMAssetSource); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, types.NewErrNotFound("MDM asset source not found")
		}
		return nil, fmt.Errorf("failed to get MDM asset source: %w", err)
	}
	return &source, nil
}

func convertMDMAssetSource(source v1.MDMAssetSource) types.MDMAssetSource {
	return types.MDMAssetSource{
		Metadata: MetadataFrom(&source),
		MDMAssetSourceManifest: types.MDMAssetSourceManifest{
			Source: mdmassets.RedactSource(source.Spec.Source),
		},
		LastSyncTime: *types.NewTime(source.Status.LastSyncTime.Time),
		IsSyncing:    source.Annotations[v1.MDMAssetSourceSyncAnnotation] == "true",
		SyncError:    source.Status.SyncError,
		LatestDigest: source.Status.LatestDigest,
	}
}

func convertMDMAsset(asset v1.MDMAsset) types.MDMAsset {
	return types.MDMAsset{
		Metadata: MetadataFrom(&asset),
		Digest:   asset.Spec.Digest,
		MDMAssetManifest: types.MDMAssetManifest{
			SchemaVersion:     asset.Spec.SchemaVersion,
			ObotSentryVersion: asset.Spec.ObotSentryVersion,
			Fields:            asset.Spec.Fields.Raw,
			Platforms:         asset.Spec.Platforms,
			Configurations:    asset.Spec.Configurations,
		},
	}
}
