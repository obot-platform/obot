package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
	"github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
)

type ThreadHandler struct {
	workspaceClient *wclient.Client
}

func NewThreadHandler(wc *wclient.Client) *ThreadHandler {
	return &ThreadHandler{
		workspaceClient: wc,
	}
}

func convertThread(thread v1.Thread) types.Thread {
	return types.Thread{
		ID:            thread.Name,
		Created:       thread.CreationTimestamp.Time,
		Description:   thread.Status.Description,
		AgentID:       thread.Spec.AgentName,
		Input:         thread.Spec.Input,
		LastRunName:   thread.Status.LastRunName,
		LastRunState:  thread.Status.LastRunState,
		LastRunOutput: thread.Status.LastRunOutput,
		LastRunError:  thread.Status.LastRunError,
	}
}

func (a *ThreadHandler) List(req api.Context) error {
	var (
		agentName  = req.PathValue("agent")
		threadList v1.ThreadList
	)
	if err := req.List(&threadList); err != nil {
		return err
	}

	var resp types.ThreadList
	for _, thread := range threadList.Items {
		if agentName == "" || thread.Spec.AgentName == agentName {
			resp.Items = append(resp.Items, convertThread(thread))
		}
	}

	return req.Write(resp)
}
func (a *ThreadHandler) Files(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)
	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, thread.Spec.WorkspaceID)
}

func (a *ThreadHandler) UploadFile(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)
	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	return uploadFile(req.Context(), req, a.workspaceClient, thread.Spec.WorkspaceID)
}

func (a *ThreadHandler) DeleteFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		filename = req.PathValue("file")
		thread   v1.Thread
	)

	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.workspaceClient, thread.Spec.WorkspaceID, filename)
}

func (a *ThreadHandler) Knowledge(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)
	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, thread.Spec.KnowledgeWorkspaceID)
}

func (a *ThreadHandler) UploadKnowledge(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)
	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.workspaceClient, thread.Spec.KnowledgeWorkspaceID); err != nil {
		return err
	}

	thread.Status.IngestKnowledge = true
	thread.Status.HasKnowledge = true
	return req.Storage.Status().Update(req.Context(), &thread)
}

func (a *ThreadHandler) DeleteKnowledge(req api.Context) error {
	var (
		id       = req.PathValue("id")
		filename = req.PathValue("file")
		thread   v1.Thread
	)

	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	if err := deleteFile(req.Context(), req, a.workspaceClient, thread.Spec.KnowledgeWorkspaceID, filename); err != nil {
		return err
	}

	thread.Status.IngestKnowledge = true
	return req.Storage.Status().Update(req.Context(), &thread)
}

func (a *ThreadHandler) IngestKnowledge(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)
	if err := req.Get(&thread, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	files, err := a.workspaceClient.Ls(req.Context(), thread.Spec.KnowledgeWorkspaceID)
	if err != nil {
		return err
	}

	req.WriteHeader(http.StatusNoContent)

	if len(files) == 0 && !thread.Status.HasKnowledge {
		return nil
	}

	thread.Status.IngestKnowledge = true
	return req.Storage.Status().Update(req.Context(), &thread)
}
