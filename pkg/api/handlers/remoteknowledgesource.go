package handlers

import (
	"fmt"
	"net/http"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createRemoteKnowledgeSource(req api.Context, agentName string) error {
	var agent v1.Agent
	if err := req.Get(&agent, agentName); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return fmt.Errorf("agent %q knowledge set is not created yet", agentName)
	}

	var input types.RemoteKnowledgeSourceManifest
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	source := v1.RemoteKnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    req.Namespace(),
			GenerateName: system.RemoteKnowledgeSourcePrefix,
		},
		Spec: v1.RemoteKnowledgeSourceSpec{
			KnowledgeSetName: agent.Status.KnowledgeSetNames[0],
			Manifest:         input,
		},
	}

	if err := req.Create(&source); err != nil {
		return fmt.Errorf("failed to create RemoteKnowledgeSource: %w", err)
	}

	if err := createSyncRequest(req, source); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	return req.Write(convertRemoteKnowledgeSource(agentName, source))
}

func updateRemoteKnowledgeSource(req api.Context, remoteKnowledgeSourceName, agentName string) error {
	var agent v1.Agent
	if err := req.Get(&agent, agentName); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return fmt.Errorf("agent %q knowledge set is not created yet", agentName)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, remoteKnowledgeSourceName); err != nil {
		return err
	}

	if remoteKnowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return fmt.Errorf("RemoteKnowledgeSource %q does not belong to agent %q", remoteKnowledgeSourceName, agentName)
	}

	var manifest types.RemoteKnowledgeSourceManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	configChanged := checkConfigChanged(manifest.RemoteKnowledgeSourceInput, remoteKnowledgeSource)
	remoteKnowledgeSource.Spec.Manifest = manifest

	if err := req.Update(&remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to update RemoteKnowledgeSource: %w", err)
	}

	if configChanged {
		if err := createSyncRequest(req, remoteKnowledgeSource); err != nil {
			return fmt.Errorf("failed to create sync request: %w", err)
		}
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func reSyncRemoteKnowledgeSource(req api.Context, linksID, agentName string) error {
	var agent v1.Agent
	if err := req.Get(&agent, agentName); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return fmt.Errorf("agent %q knowledge set is not created yet", agentName)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, linksID); err != nil {
		return fmt.Errorf("failed to get RemoteKnowledgeSource with id %s: %w", linksID, err)
	}

	if remoteKnowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return fmt.Errorf("RemoteKnowledgeSource %q does not belong to agent %q", linksID, agentName)
	}

	if err := createSyncRequest(req, remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func createSyncRequest(req api.Context, remoteKnowledgeSource v1.RemoteKnowledgeSource) error {
	if err := req.Storage.Create(req.Context(), &v1.SyncUploadRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SyncRequestPrefix,
			Namespace:    remoteKnowledgeSource.Namespace,
		},
		Spec: v1.SyncUploadRequestSpec{
			RemoteKnowledgeSourceName: remoteKnowledgeSource.Name,
		},
	}); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	return nil
}

func getRemoteKnowledgeSourceForParent(req api.Context, agentName string) error {
	var agent v1.Agent
	if err := req.Get(&agent, agentName); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return req.Write(types.RemoteKnowledgeSourceList{Items: []types.RemoteKnowledgeSource{}})
	}

	var remoteKnowledgeSourceList v1.RemoteKnowledgeSourceList
	if err := req.Storage.List(req.Context(), &remoteKnowledgeSourceList,
		client.InNamespace(req.Namespace()), client.MatchingFields{
			"spec.knowledgeSetName": agent.Status.KnowledgeSetNames[0],
		}); err != nil {
		return err
	}

	resp := make([]types.RemoteKnowledgeSource, 0, len(remoteKnowledgeSourceList.Items))
	for _, source := range remoteKnowledgeSourceList.Items {
		resp = append(resp, convertRemoteKnowledgeSource(agentName, source))
	}

	return req.Write(types.RemoteKnowledgeSourceList{Items: resp})
}

func deleteRemoteKnowledgeSource(req api.Context, remoteKnowledgeSourceID, agentName string) error {
	var agent v1.Agent
	if err := req.Get(&agent, agentName); err != nil {
		return err
	}

	if len(agent.Status.KnowledgeSetNames) == 0 {
		return fmt.Errorf("agent %q knowledge set is not created yet", agentName)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, remoteKnowledgeSourceID); err != nil {
		return err
	}

	if remoteKnowledgeSource.Spec.KnowledgeSetName != agent.Status.KnowledgeSetNames[0] {
		return fmt.Errorf("RemoteKnowledgeSource %q does not belong to agent %q", remoteKnowledgeSourceID, agentName)
	}

	if err := req.Delete(&v1.RemoteKnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remoteKnowledgeSourceID,
			Namespace: req.Namespace(),
		},
	}); err != nil {
		return fmt.Errorf("failed to delete RemoteKnowledgeSource: %w", err)
	}

	return nil
}

func convertRemoteKnowledgeSource(agentName string, remoteKnowledgeSource v1.RemoteKnowledgeSource) types.RemoteKnowledgeSource {
	return types.RemoteKnowledgeSource{
		Metadata:                      MetadataFrom(&remoteKnowledgeSource),
		RemoteKnowledgeSourceManifest: remoteKnowledgeSource.Spec.Manifest,
		AgentID:                       agentName,
		State:                         remoteKnowledgeSource.Status.State,
		ThreadID:                      remoteKnowledgeSource.Status.ThreadName,
		RunID:                         remoteKnowledgeSource.Status.RunName,
		Status:                        remoteKnowledgeSource.Status.Status,
		Error:                         remoteKnowledgeSource.Status.Error,
	}
}

func checkConfigChanged(input types.RemoteKnowledgeSourceInput, remoteKnowledgeSource v1.RemoteKnowledgeSource) bool {
	if input.OneDriveConfig != nil && remoteKnowledgeSource.Spec.Manifest.OneDriveConfig != nil {
		return !equality.Semantic.DeepEqual(*input.OneDriveConfig, *remoteKnowledgeSource.Spec.Manifest.OneDriveConfig)
	}

	if remoteKnowledgeSource.Spec.Manifest.SourceType == types.RemoteKnowledgeSourceTypeNotion {
		// we never resync notion on update, this is because by default we sync every page that it has access to
		return false
	}

	if input.WebsiteCrawlingConfig != nil && remoteKnowledgeSource.Spec.Manifest.WebsiteCrawlingConfig != nil {
		return !equality.Semantic.DeepEqual(*input.WebsiteCrawlingConfig, *remoteKnowledgeSource.Spec.Manifest.WebsiteCrawlingConfig)
	}

	return true
}
