package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SystemMCPServerSourcesHandler struct{}

func NewSystemMCPServerSourcesHandler() *SystemMCPServerSourcesHandler {
	return &SystemMCPServerSourcesHandler{}
}

// SystemMCPServerSourcesManifest is a singleton resource that tracks git source URLs
type SystemMCPServerSourcesManifest struct {
	SourceURLs []string `json:"sourceURLs"`
}

func (h *SystemMCPServerSourcesHandler) Get(req api.Context) error {
	var sources v1.SystemMCPServerSources
	if err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.SystemMCPServerSourcesName,
	}, &sources); err != nil {
		return err
	}

	return req.Write(types.SystemMCPServerSources{
		Metadata:   MetadataFrom(&sources),
		SourceURLs: sources.Spec.SourceURLs,
		LastSynced: *types.NewTime(sources.Status.LastSyncTime.Time),
		SyncErrors: sources.Status.SyncErrors,
		IsSyncing:  sources.Status.IsSyncing,
	})
}

func (h *SystemMCPServerSourcesHandler) Update(req api.Context) error {
	var input SystemMCPServerSourcesManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	// Validate URLs
	for _, sourceURL := range input.SourceURLs {
		if sourceURL == "" {
			return types.NewErrBadRequest("source URL cannot be empty")
		}
	}

	var sources v1.SystemMCPServerSources
	if err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.SystemMCPServerSourcesName,
	}, &sources); err != nil {
		return err
	}

	sources.Spec.SourceURLs = input.SourceURLs

	if err := req.Storage.Update(req.Context(), &sources); err != nil {
		return err
	}

	return req.Write(types.SystemMCPServerSources{
		Metadata:   MetadataFrom(&sources),
		SourceURLs: sources.Spec.SourceURLs,
		LastSynced: *types.NewTime(sources.Status.LastSyncTime.Time),
		SyncErrors: sources.Status.SyncErrors,
		IsSyncing:  sources.Status.IsSyncing,
	})
}

func (h *SystemMCPServerSourcesHandler) Refresh(req api.Context) error {
	var sources v1.SystemMCPServerSources
	if err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.SystemMCPServerSourcesName,
	}, &sources); err != nil {
		return err
	}

	// Trigger sync by updating annotation
	if sources.Annotations == nil {
		sources.Annotations = make(map[string]string)
	}
	sources.Annotations["obot.ai/force-sync"] = "true"

	if err := req.Storage.Update(req.Context(), &sources); err != nil {
		return err
	}

	return req.Write(types.SystemMCPServerSources{
		Metadata:   MetadataFrom(&sources),
		SourceURLs: sources.Spec.SourceURLs,
		LastSynced: *types.NewTime(sources.Status.LastSyncTime.Time),
		SyncErrors: sources.Status.SyncErrors,
		IsSyncing:  sources.Status.IsSyncing,
	})
}
