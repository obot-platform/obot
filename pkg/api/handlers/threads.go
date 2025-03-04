package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DefaultMaxUserThreadTools = 5

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
	var env []string
	for _, e := range thread.Spec.Env {
		if e.Existing && e.Value == "" {
			env = append(env, e.Name)
		}
	}
	return types.Thread{
		Metadata:        MetadataFrom(&thread),
		ThreadManifest:  thread.Spec.Manifest,
		AgentID:         thread.Spec.AgentName,
		WorkflowID:      thread.Spec.WorkflowName,
		WebhookID:       thread.Spec.WebhookName,
		EmailReceiverID: thread.Spec.EmailReceiverName,
		LastRunID:       thread.Status.LastRunName,
		CurrentRunID:    thread.Status.CurrentRunName,
		State:           state,
		ProjectID:       thread.Spec.ParentThreadName,
		UserID:          thread.Spec.UserID,
		Abort:           thread.Spec.Abort,
		SystemTask:      thread.Spec.SystemTask,
		Ephemeral:       thread.Spec.Ephemeral,
		Project:         thread.Spec.Project,
		Env:             env,
	}
}

func (a *ThreadHandler) Abort(req api.Context) error {
	var (
		id     = req.PathValue("id")
		thread v1.Thread
	)

	if err := req.Get(&thread, id); err != nil {
		return err
	}

	if err := abortThread(req, &thread); err != nil {
		return err
	}

	return req.Write(thread)
}

func (a *ThreadHandler) Events(req api.Context) error {
	var (
		id              = req.PathValue("id")
		follow          = req.URL.Query().Get("follow") == "true"
		followWorkflows = req.URL.Query().Get("followWorkflows") == "true"
		runID           = req.URL.Query().Get("runID")
		maxRunString    = req.URL.Query().Get("maxRuns")
		maxRuns         int
		err             error
		waitForThread   = req.URL.Query().Get("waitForThread") == "true"
	)

	if runID == "" {
		runID = req.Request.Header.Get("Last-Event-ID")
	}

	if maxRunString != "" {
		maxRuns, err = strconv.Atoi(maxRunString)
		if err != nil {
			return types.NewErrBadRequest("maxEvents must be an integer")
		}
	} else {
		maxRuns = 25
	}

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		Follow:                   follow,
		FollowWorkflowExecutions: followWorkflows,
		History:                  runID == "",
		LastRunName:              strings.TrimSuffix(runID, ":after"),
		MaxRuns:                  maxRuns,
		After:                    strings.HasSuffix(runID, ":after"),
		ThreadName:               id,
		WaitForThread:            waitForThread,
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

	// Don't allow update of tools here, do it with the /tools endpoint
	newThread.Tools = existing.Spec.Manifest.Tools

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

func (a *ThreadHandler) Knowledge(req api.Context) error {
	var (
		threadID = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	if thread.Status.SharedKnowledgeSetName == "" {
		return req.Write(types.KnowledgeFileList{Items: []types.KnowledgeFile{}})
	}

	return listKnowledgeFiles(req, "", thread.Name, thread.Status.SharedKnowledgeSetName, nil)
}

func (a *ThreadHandler) GetKnowledgeFile(req api.Context) error {
	var (
		threadID = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}
	return getKnowledgeFile(req, a.gptscript, &thread, nil, req.PathValue("file"))
}

func (a *ThreadHandler) UploadKnowledge(req api.Context) error {
	var (
		threadID = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	if thread.Status.SharedKnowledgeSetName == "" {
		return types.NewErrHTTP(http.StatusTooEarly, "knowledge set is not available yet")
	}

	ws, err := getWorkspaceFromKnowledgeSet(req, thread.Status.SharedKnowledgeSetName)
	if err != nil {
		return err
	}

	return uploadKnowledgeToWorkspace(req, a.gptscript, ws, "", thread.Name, thread.Status.SharedKnowledgeSetName)
}

func (a *ThreadHandler) DeleteKnowledge(req api.Context) error {
	var (
		threadID = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	if thread.Status.SharedKnowledgeSetName == "" {
		return types.NewErrHTTP(http.StatusTooEarly, fmt.Sprintf("thread %q knowledge set is not created yet", thread.Name))
	}

	return deleteKnowledge(req, req.PathValue("file"), thread.Status.SharedKnowledgeSetName)
}

func getThreadDBWorkspaceID(req api.Context, thread v1.Thread) (string, error) {
	if thread.IsUserThread() {
		if err := req.Get(&thread, thread.Spec.ParentThreadName); err != nil {
			return "", err
		}
	}

	if thread.Status.LocalWorkspaceName == "" {
		return "", nil
	}

	var ws v1.Workspace
	if err := req.Get(&ws, thread.Status.LocalWorkspaceName); err != nil {
		return "", err
	}

	return ws.Status.WorkspaceID, nil
}

func (a *ThreadHandler) Tables(req api.Context) error {
	var (
		threadID = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	wsID, err := getThreadDBWorkspaceID(req, thread)
	if err != nil {
		return err
	}

	if wsID == "" {
		return req.Write(types.TableList{Items: []types.Table{}})
	}

	return listTablesInWorkspace(req, a.gptscript, wsID)
}

func (a *ThreadHandler) TableRows(req api.Context) error {
	var (
		threadID  = req.PathValue("id")
		tableName = req.PathValue("table")
	)

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	wsID, err := getThreadDBWorkspaceID(req, thread)
	if err != nil {
		return err
	}

	if wsID == "" {
		return req.Write(types.TableRowList{Items: []types.TableRow{}})
	}

	return listTableRows(req, a.gptscript, wsID, tableName)
}
