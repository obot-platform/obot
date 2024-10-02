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

func createOneDriveLinks(req api.Context, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var links []string
	if err := req.Read(&links); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	oneDriveLinks := &v1.OneDriveLinks{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "ol1",
			Namespace:    req.Namespace(),
		},
		Spec: v1.OnedriveLinksSpec{
			SharedLinks: links,
		},
	}

	switch parentObj.(type) {
	case *v1.Agent:
		oneDriveLinks.Spec.AgentName = parentName
	case *v1.Workflow:
		oneDriveLinks.Spec.WorkflowName = parentName
	default:
		return fmt.Errorf("unknown parent object type: %T", parentObj)
	}

	if err := req.Create(oneDriveLinks); err != nil {
		return fmt.Errorf("failed to create OneDrive links: %w", err)
	}

	if err := createSyncRequest(req, *oneDriveLinks); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	return req.Write(convertOneDriveLinks(*oneDriveLinks))
}

func updateOneDriveLinks(req api.Context, linksID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var oneDriveLinks v1.OneDriveLinks
	if err := req.Get(&oneDriveLinks, linksID); err != nil {
		return fmt.Errorf("failed to get OneDrive links with id %s: %w", linksID, err)
	}

	if oneDriveLinks.Spec.AgentName != parentName && oneDriveLinks.Spec.WorkflowName != parentName {
		return fmt.Errorf("OneDrive links agent name %q does not match provided agent name %q", oneDriveLinks.Spec.AgentName, parentName)
	}

	var links []string
	if err := req.Read(&links); err != nil {
		return fmt.Errorf("failed to decode request body: %w", err)
	}

	oneDriveLinks.Spec.SharedLinks = links

	if err := req.Update(&oneDriveLinks); err != nil {
		return fmt.Errorf("failed to update OneDrive links: %w", err)
	}

	if err := createSyncRequest(req, oneDriveLinks); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func reSyncOneDriveLinks(req api.Context, linksID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var oneDriveLinks v1.OneDriveLinks
	if err := req.Get(&oneDriveLinks, linksID); err != nil {
		return fmt.Errorf("failed to get OneDrive links with id %s: %w", linksID, err)
	}

	if oneDriveLinks.Spec.AgentName != parentName && oneDriveLinks.Spec.WorkflowName != parentName {
		return fmt.Errorf("OneDrive links agent name %q does not match provided agent name %q", oneDriveLinks.Spec.AgentName, parentName)
	}

	if err := createSyncRequest(req, oneDriveLinks); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func createSyncRequest(req api.Context, oneDriveLinks v1.OneDriveLinks) error {
	if err := req.Storage.Create(req.Context(), &v1.SyncUploadRequest{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SyncRequestPrefix,
			Namespace:    oneDriveLinks.Namespace,
		},
		Spec: v1.SyncUploadRequestSpec{
			UploadName: oneDriveLinks.Name,
		},
	}); err != nil {
		return fmt.Errorf("failed to create sync request: %w", err)
	}

	return nil
}

func getOneDriveLinksForParent(req api.Context, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var oneDriveLinks v1.OneDriveLinksList
	if err := req.List(&oneDriveLinks); err != nil {
		return fmt.Errorf("failed to get OneDrive links with id %s: %w", parentName, err)
	}

	resp := make([]types.OneDriveLinks, 0, len(oneDriveLinks.Items))
	for _, link := range oneDriveLinks.Items {
		if link.Spec.WorkflowName == parentName || link.Spec.AgentName == parentName {
			resp = append(resp, convertOneDriveLinks(link))
		}
	}

	return req.Write(types.OneDriveLinksList{Items: resp})
}

func deleteOneDriveLinks(req api.Context, linksID, parentName string, parentObj client.Object) error {
	if err := req.Get(parentObj, parentName); err != nil {
		return fmt.Errorf("failed to get parent with id %s: %w", parentName, err)
	}

	var oneDriveLinks v1.OneDriveLinks
	if err := req.Get(&oneDriveLinks, linksID); types.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get OneDrive links with id %s: %w", linksID, err)
	}

	if oneDriveLinks.Spec.AgentName != parentName && oneDriveLinks.Spec.WorkflowName != parentName {
		return fmt.Errorf("OneDrive links name %q does not match provided agent name %q", oneDriveLinks.Spec.AgentName, parentName)
	}

	if err := req.Delete(&oneDriveLinks); err != nil {
		return fmt.Errorf("failed to delete OneDrive links: %w", err)
	}

	return nil
}

func convertOneDriveLinks(oneDriveLink v1.OneDriveLinks) types.OneDriveLinks {
	return types.OneDriveLinks{
		Metadata:    MetadataFrom(&oneDriveLink),
		AgentID:     oneDriveLink.Spec.AgentName,
		WorkflowID:  oneDriveLink.Spec.WorkflowName,
		SharedLinks: oneDriveLink.Spec.SharedLinks,
		ThreadID:    oneDriveLink.Status.ThreadName,
		RunID:       oneDriveLink.Status.RunName,
		Status:      oneDriveLink.Status.Status,
		Error:       oneDriveLink.Status.Error,
		Folders:     oneDriveLink.Status.Folders,
	}
}
