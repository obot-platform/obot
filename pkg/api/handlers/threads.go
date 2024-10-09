package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/events"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	wclient "github.com/otto8-ai/workspace-provider/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ThreadHandler struct {
	workspaceClient *wclient.Client
	events          *events.Emitter
}

func NewThreadHandler(wc *wclient.Client, events *events.Emitter) *ThreadHandler {
	return &ThreadHandler{
		workspaceClient: wc,
		events:          events,
	}
}

func convertThread(thread v1.Thread) types.Thread {
	var state = string(thread.Status.LastRunState)
	if thread.Status.WorkflowState != "" {
		state = string(thread.Status.WorkflowState)
	}
	parent := thread.Spec.ParentThreadName
	if parent == "" {
		parent = thread.Status.PreviousThreadName
	}
	return types.Thread{
		Metadata: MetadataFrom(&thread),
		ThreadManifest: types.ThreadManifest{
			Description: thread.Spec.Manifest.Description,
			Tools:       thread.Spec.Manifest.Tools,
		},
		AgentID:        thread.Spec.AgentName,
		WorkflowID:     thread.Spec.WorkflowName,
		LastRunID:      thread.Status.LastRunName,
		CurrentRunID:   thread.Status.CurrentRunName,
		State:          state,
		ParentThreadID: parent,
	}
}

func (a *ThreadHandler) Events(req api.Context) error {
	var (
		id     = req.PathValue("id")
		follow = req.URL.Query().Get("follow") == "true"
		runID  = req.URL.Query().Get("runID")
		thread v1.Thread
	)

	if err := req.Get(&thread, id); err != nil {
		return err
	}

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		Follow:      follow,
		History:     runID == "",
		LastRunName: runID,
		ThreadName:  thread.Name,
	})
	if err != nil {
		return err
	}

	return req.WriteEvents(events)
}

func (a *ThreadHandler) ByID(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)

	if err := req.Get(&thread, id); err != nil {
		return err
	}

	return req.Write(convertThread(thread))
}

func (a *ThreadHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)
	return req.Delete(&v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *ThreadHandler) Update(req api.Context) error {
	var (
		id        = req.PathValue("id")
		newThread types.ThreadManifest
		existing  v1.Thread
	)

	if err := req.Get(&existing, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	if err := req.Read(&newThread); err != nil {
		return err
	}

	existing.Spec.Manifest = newThread
	if err := req.Update(&existing); err != nil {
		return err
	}

	return req.Write(convertThread(existing))
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
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if !workspace.Spec.IsKnowledge {
			return listFileFromWorkspace(req.Context(), req, a.workspaceClient, workspace)
		}
	}

	return fmt.Errorf("no workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) UploadFile(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if !workspace.Spec.IsKnowledge {
			if err := uploadFileToWorkspace(req.Context(), req, a.workspaceClient, workspace); err != nil {
				return err
			}

			req.WriteHeader(http.StatusCreated)
			return nil
		}
	}

	return fmt.Errorf("no workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) DeleteFile(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if !workspace.Spec.IsKnowledge {
			return deleteFileFromWorkspaceID(req.Context(), req, a.workspaceClient, workspace.Spec.WorkspaceID)
		}
	}

	return fmt.Errorf("no workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) Knowledge(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if workspace.Spec.IsKnowledge {
			return listKnowledgeFilesFromWorkspace(req, workspace)
		}
	}

	return fmt.Errorf("no knowledge workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) UploadKnowledge(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if workspace.Spec.IsKnowledge {
			return uploadKnowledgeToWorkspace(req, a.workspaceClient, workspace)
		}
	}

	return fmt.Errorf("no knowledge workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) DeleteKnowledge(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if workspace.Spec.IsKnowledge {
			return deleteKnowledgeFromWorkspace(req, req.PathValue("file"), workspace)
		}
	}

	return fmt.Errorf("no knowledge workspace found for thread %s", req.PathValue("id"))
}

func (a *ThreadHandler) IngestKnowledge(req api.Context) error {
	var workspaces v1.WorkspaceList
	if err := req.Storage.List(req.Context(), &workspaces, &client.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.threadName": req.PathValue("id"),
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if workspace.Spec.IsKnowledge {
			return ingestKnowledgeInWorkspace(req, a.workspaceClient, workspace)
		}
	}

	return fmt.Errorf("no knowledge workspace found for thread %s", req.PathValue("id"))
}
