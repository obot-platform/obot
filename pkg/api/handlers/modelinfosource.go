package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// ModelInfoSourceHandler serves the default ModelInfoSource.
type ModelInfoSourceHandler struct{}

func NewModelInfoSourceHandler() *ModelInfoSourceHandler {
	return nil
}

// Get returns the default ModelInfoSource.
func (*ModelInfoSourceHandler) Get(req api.Context) error {
	var source v1.ModelInfoSource
	if err := req.Get(&source, system.DefaultModelInfoSource); err != nil {
		return fmt.Errorf("failed to get model info source: %w", err)
	}
	return req.Write(convertModelInfoSource(source))
}

// Refresh requests a ModelInfoSource sync.
func (*ModelInfoSourceHandler) Refresh(req api.Context) error {
	var source v1.ModelInfoSource
	if err := req.Get(&source, system.DefaultModelInfoSource); err != nil {
		return fmt.Errorf("failed to get model info source: %w", err)
	}

	if source.Annotations == nil {
		source.Annotations = make(map[string]string)
	}
	source.Annotations[v1.ModelInfoSourceSyncAnnotation] = "true"

	return req.Update(&source)
}

func convertModelInfoSource(source v1.ModelInfoSource) types.ModelInfoSource {
	return types.ModelInfoSource{
		Metadata: MetadataFrom(&source),
		ModelInfoSourceManifest: types.ModelInfoSourceManifest{
			URL: source.Spec.Manifest.URL,
		},
		LastSynced: *types.NewTime(source.Status.LastSyncTime.Time),
		SyncError:  source.Status.SyncError,
		IsSyncing:  source.Annotations[v1.ModelInfoSourceSyncAnnotation] == "true",
		ModelCount: source.Status.ModelCount,
	}
}
