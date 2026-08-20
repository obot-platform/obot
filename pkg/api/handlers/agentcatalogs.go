package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentcatalog"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type AgentCatalogHandler struct {
	devMode bool
}

func NewAgentCatalogHandler(devMode bool) *AgentCatalogHandler {
	return &AgentCatalogHandler{devMode: devMode}
}

func (*AgentCatalogHandler) List(req api.Context) error {
	var list v1.AgentCatalogList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list agent catalogs: %w", err)
	}

	items := make([]types.AgentCatalog, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertAgentCatalog(item))
	}

	return req.Write(types.AgentCatalogList{Items: items})
}

func (*AgentCatalogHandler) Get(req api.Context) error {
	var source v1.AgentCatalog
	if err := req.Get(&source, req.PathValue("agent_catalog_id")); err != nil {
		return fmt.Errorf("failed to get agent catalog: %w", err)
	}

	return req.Write(convertAgentCatalog(source))
}

func (h *AgentCatalogHandler) Create(req api.Context) error {
	manifest, err := h.readAndValidateAgentCatalogManifest(req)
	if err != nil {
		return err
	}

	source := v1.AgentCatalog{
		GenerateName: system.AgentCatalogPrefix,
		Namespace:    req.Namespace(),
		Spec: v1.AgentCatalogSpec{
			AgentCatalogManifest: *manifest,
		},
	}

	if err := req.Create(&source); err != nil {
		return fmt.Errorf("failed to create agent catalog: %w", err)
	}

	return req.WriteCreated(convertAgentCatalog(source))
}

func (h *AgentCatalogHandler) Update(req api.Context) error {
	manifest, err := h.readAndValidateAgentCatalogManifest(req)
	if err != nil {
		return err
	}

	var source v1.AgentCatalog
	if err := req.Get(&source, req.PathValue("agent_catalog_id")); err != nil {
		return fmt.Errorf("failed to get agent catalog: %w", err)
	}

	source.Spec.AgentCatalogManifest = *manifest
	if err := req.Update(&source); err != nil {
		return fmt.Errorf("failed to update agent catalog: %w", err)
	}

	return req.Write(convertAgentCatalog(source))
}

func (*AgentCatalogHandler) Delete(req api.Context) error {
	return req.Delete(&v1.AgentCatalog{
		Name:      req.PathValue("agent_catalog_id"),
		Namespace: req.Namespace(),
	})
}

// Refresh asks the controller to sync now by setting the force-sync annotation,
// rather than doing any work itself.
func (*AgentCatalogHandler) Refresh(req api.Context) error {
	var source v1.AgentCatalog
	if err := req.Get(&source, req.PathValue("agent_catalog_id")); err != nil {
		return fmt.Errorf("failed to get agent catalog: %w", err)
	}

	if source.Annotations == nil {
		source.Annotations = map[string]string{}
	}
	source.Annotations[v1.AgentCatalogSyncAnnotation] = "true"

	if err := req.Update(&source); err != nil {
		return fmt.Errorf("failed to refresh agent catalog: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *AgentCatalogHandler) readAndValidateAgentCatalogManifest(req api.Context) (*types.AgentCatalogManifest, error) {
	var manifest types.AgentCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return nil, types.NewErrBadRequest("failed to read agent catalog manifest: %v", err)
	}

	manifest.DisplayName = strings.TrimSpace(manifest.DisplayName)
	manifest.RepoURL = strings.TrimSpace(manifest.RepoURL)
	manifest.Ref = strings.TrimSpace(manifest.Ref)

	if err := agentcatalog.Validate(manifest, h.devMode); err != nil {
		return nil, types.NewErrBadRequest("invalid agent catalog manifest: %v", err)
	}

	return &manifest, nil
}

func convertAgentCatalog(source v1.AgentCatalog) types.AgentCatalog {
	return types.AgentCatalog{
		Metadata:               MetadataFrom(&source),
		AgentCatalogManifest:   source.Spec.AgentCatalogManifest,
		LastSyncTime:           *types.NewTime(source.Status.LastSyncTime.Time),
		IsSyncing:              source.Status.IsSyncing,
		SyncError:              source.Status.SyncError,
		ResolvedCommitSHA:      source.Status.ResolvedCommitSHA,
		DiscoveredAgentCount:   source.Status.DiscoveredAgentCount,
		DiscoveredHarnessCount: source.Status.DiscoveredHarnessCount,
	}
}
