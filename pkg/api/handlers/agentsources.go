package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AgentSourceHandler struct{}

func NewAgentSourceHandler() *AgentSourceHandler {
	return nil
}

func (*AgentSourceHandler) List(req api.Context) error {
	var list v1.AgentSourceList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list agent sources: %w", err)
	}

	items := make([]types.AgentSource, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertAgentSource(item))
	}

	return req.Write(types.AgentSourceList{Items: items})
}

func (*AgentSourceHandler) Get(req api.Context) error {
	var source v1.AgentSource
	if err := req.Get(&source, req.PathValue("agent_source_id")); err != nil {
		return fmt.Errorf("failed to get agent source: %w", err)
	}

	return req.Write(convertAgentSource(source))
}

func (*AgentSourceHandler) Create(req api.Context) error {
	manifest, err := readAndValidateAgentSourceManifest(req)
	if err != nil {
		return err
	}

	source := v1.AgentSource{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AgentSourcePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AgentSourceSpec{
			AgentSourceManifest: *manifest,
		},
	}

	if err := req.Create(&source); err != nil {
		return fmt.Errorf("failed to create agent source: %w", err)
	}

	return req.WriteCreated(convertAgentSource(source))
}

func (*AgentSourceHandler) Update(req api.Context) error {
	manifest, err := readAndValidateAgentSourceManifest(req)
	if err != nil {
		return err
	}

	var source v1.AgentSource
	if err := req.Get(&source, req.PathValue("agent_source_id")); err != nil {
		return fmt.Errorf("failed to get agent source: %w", err)
	}

	source.Spec.AgentSourceManifest = *manifest
	if err := req.Update(&source); err != nil {
		return fmt.Errorf("failed to update agent source: %w", err)
	}

	return req.Write(convertAgentSource(source))
}

func (*AgentSourceHandler) Delete(req api.Context) error {
	return req.Delete(&v1.AgentSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("agent_source_id"),
			Namespace: req.Namespace(),
		},
	})
}

// Refresh asks the controller to sync now by setting the force-sync annotation,
// rather than doing any work itself.
func (*AgentSourceHandler) Refresh(req api.Context) error {
	var source v1.AgentSource
	if err := req.Get(&source, req.PathValue("agent_source_id")); err != nil {
		return fmt.Errorf("failed to get agent source: %w", err)
	}

	if source.Annotations == nil {
		source.Annotations = map[string]string{}
	}
	source.Annotations[v1.AgentSourceSyncAnnotation] = "true"

	if err := req.Update(&source); err != nil {
		return fmt.Errorf("failed to refresh agent source: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func readAndValidateAgentSourceManifest(req api.Context) (*types.AgentSourceManifest, error) {
	var manifest types.AgentSourceManifest
	if err := req.Read(&manifest); err != nil {
		return nil, types.NewErrBadRequest("failed to read agent source manifest: %v", err)
	}

	manifest.DisplayName = strings.TrimSpace(manifest.DisplayName)
	manifest.RepoURL = strings.TrimSpace(manifest.RepoURL)
	manifest.Ref = strings.TrimSpace(manifest.Ref)

	if err := manifest.Validate(); err != nil {
		return nil, types.NewErrBadRequest("invalid agent source manifest: %v", err)
	}

	return &manifest, nil
}

func convertAgentSource(source v1.AgentSource) types.AgentSource {
	return types.AgentSource{
		Metadata:             MetadataFrom(&source),
		AgentSourceManifest:  source.Spec.AgentSourceManifest,
		LastSyncTime:         *types.NewTime(source.Status.LastSyncTime.Time),
		IsSyncing:            source.Status.IsSyncing,
		SyncError:            source.Status.SyncError,
		ResolvedCommitSHA:    source.Status.ResolvedCommitSHA,
		DiscoveredAgentCount: source.Status.DiscoveredAgentCount,
	}
}
