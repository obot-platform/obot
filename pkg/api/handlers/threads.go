package handlers

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/events"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ThreadHandler struct {
	gptscript *gptscript.GPTScript
	events    *events.Emitter
}

func NewThreadHandler(gClient *gptscript.GPTScript, events *events.Emitter) *ThreadHandler {
	return &ThreadHandler{
		gptscript: gClient,
		events:    events,
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
		id            = req.PathValue("id")
		follow        = req.URL.Query().Get("follow") == "true"
		runID         = req.URL.Query().Get("runID")
		maxRunString  = req.URL.Query().Get("maxRuns")
		maxRuns       int
		err           error
		thread        v1.Thread
		waitForThread = req.URL.Query().Get("waitForThread") == "true"
	)

	if id == "user" {
		id = system.ThreadPrefix + req.User.GetUID()
	}

	if maxRunString != "" {
		maxRuns, err = strconv.Atoi(maxRunString)
		if err != nil {
			return api.NewErrBadRequest("maxEvents must be an integer")
		}
	} else {
		maxRuns = 20
	}

	if err := req.Get(&thread, id); err != nil {
		return err
	}

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		Follow:        follow,
		History:       runID == "",
		LastRunName:   strings.TrimSuffix(runID, ":after"),
		MaxRuns:       maxRuns,
		After:         strings.HasSuffix(runID, ":after"),
		ThreadName:    thread.Name,
		WaitForThread: waitForThread,
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
		agent     v1.Agent
	)

	if err := req.Get(&existing, id); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", id, err)
	}

	if err := req.Read(&newThread); err != nil {
		return err
	}

	if existing.Spec.AgentName != "" {
		if err := req.Get(&agent, existing.Spec.AgentName); err != nil {
			return err
		}
		for _, newTool := range newThread.Tools {
			if !slices.Contains(agent.Spec.Manifest.AvailableThreadTools, newTool) {
				return api.NewErrBadRequest("tool %s is not available for agent %s", newTool, agent.Name)
			}
		}
		max := agent.Spec.Manifest.MaxThreadTools
		if max == 0 {
			max = 5
		}
		if len(newThread.Tools) > max {
			return api.NewErrBadRequest("too many tools, max %d", max)
		}
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
			return listFileFromWorkspace(req.Context(), req, a.gptscript, workspace)
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
			if err := uploadFileToWorkspace(req.Context(), req, a.gptscript, workspace); err != nil {
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
			return deleteFileFromWorkspaceID(req.Context(), req, a.gptscript, workspace.Spec.WorkspaceID)
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
			return uploadKnowledgeToWorkspace(req, a.gptscript, workspace)
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
