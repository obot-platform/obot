package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/api"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createRemoteKnowledgeSource(req api.Context, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var input types.RemoteKnowledgeSourceInput
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	remoteKnowledgeSource := &v1.RemoteKnowledgeSource{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "remote-knowledge-source-",
			Namespace:    req.Namespace(),
		},
		Spec: v1.RemoteKnowledgeSourceSpec{
			Input: input,
		},
	}

	switch parentObj.(type) {
	case *v1.Agent:
		remoteKnowledgeSource.Spec.AgentName = parentName
	case *v1.Workflow:
		remoteKnowledgeSource.Spec.WorkflowName = parentName
	default:
		return fmt.Errorf("unknown parent object type: %T", parentObj)
	}

	if err := req.Create(remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to create RemoteKnowledgeSource: %w", err)
	}

	if err := createSyncRequest(req, *remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	return req.Write(convertRemoteKnowledgeSource(*remoteKnowledgeSource))
}

func updateRemoteKnowledgeSource(req api.Context, linksID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, linksID); err != nil {
		return fmt.Errorf("failed to get RemoteKnowledgeSource with id %s: %w", linksID, err)
	}

	if remoteKnowledgeSource.Spec.AgentName != parentName && remoteKnowledgeSource.Spec.WorkflowName != parentName {
		return fmt.Errorf("RemoteKnowledgeSource agent name %q does not match provided agent name %q", remoteKnowledgeSource.Spec.AgentName, parentName)
	}

	var input types.RemoteKnowledgeSourceInput
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	remoteKnowledgeSource.Spec.Input = input

	if err := req.Update(&remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to update RemoteKnowledgeSource: %w", err)
	}

	if err := createSyncRequest(req, remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func reSyncRemoteKnowledgeSource(req api.Context, linksID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, linksID); err != nil {
		return fmt.Errorf("failed to get RemoteKnowledgeSource with id %s: %w", linksID, err)
	}

	if remoteKnowledgeSource.Spec.AgentName != parentName && remoteKnowledgeSource.Spec.WorkflowName != parentName {
		return fmt.Errorf("RemoteKnowledgeSource agent name %q does not match provided agent name %q", remoteKnowledgeSource.Spec.AgentName, parentName)
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

func getRemoteKnowledgeSourceForParent(req api.Context, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var remoteKnowledgeSourceList v1.RemoteKnowledgeSourceList
	if err := req.List(&remoteKnowledgeSourceList); err != nil {
		return fmt.Errorf("failed to get RemoteKnowledgeSource with id %s: %w", parentName, err)
	}

	resp := make([]types.RemoteKnowledgeSource, 0, len(remoteKnowledgeSourceList.Items))
	for _, source := range remoteKnowledgeSourceList.Items {
		if source.Spec.WorkflowName == parentName || source.Spec.AgentName == parentName {
			resp = append(resp, convertRemoteKnowledgeSource(source))
		}
	}

	return req.Write(types.RemoteKnowledgeSourceList{Items: resp})
}

func deleteRemoteKnowledgeSource(req api.Context, remoteKnowledgeSourceID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var remoteKnowledgeSource v1.RemoteKnowledgeSource
	if err := req.Get(&remoteKnowledgeSource, remoteKnowledgeSourceID); types.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get RemoteKnowledgeSource with id %s: %w", remoteKnowledgeSourceID, err)
	}

	if remoteKnowledgeSource.Spec.AgentName != parentName && remoteKnowledgeSource.Spec.WorkflowName != parentName {
		return fmt.Errorf("RemoteKnowledgeSource name %q does not match provided agent name %q", remoteKnowledgeSource.Spec.AgentName, parentName)
	}

	if err := req.Delete(&remoteKnowledgeSource); err != nil {
		return fmt.Errorf("failed to delete RemoteKnowledgeSource: %w", err)
	}

	return nil
}

func convertRemoteKnowledgeSource(remoteKnowledgeSource v1.RemoteKnowledgeSource) types.RemoteKnowledgeSource {
	return types.RemoteKnowledgeSource{
		Metadata:   MetadataFrom(&remoteKnowledgeSource),
		AgentID:    remoteKnowledgeSource.Spec.AgentName,
		WorkflowID: remoteKnowledgeSource.Spec.WorkflowName,
		Input:      remoteKnowledgeSource.Spec.Input,
		State:      remoteKnowledgeSource.Status.State,
		ThreadID:   remoteKnowledgeSource.Status.ThreadName,
		RunID:      remoteKnowledgeSource.Status.RunName,
		Status:     remoteKnowledgeSource.Status.Status,
		Error:      remoteKnowledgeSource.Status.Error,
	}
}
