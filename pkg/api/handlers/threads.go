package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	threadmodel "github.com/obot-platform/obot/pkg/thread"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultMaxUserThreadTools = 100

type ThreadHandler struct {
	dispatcher *dispatcher.Dispatcher
	events     *events.Emitter
	mapHelper  *modelaccesspolicy.Helper
}

func NewThreadHandler(dispatcher *dispatcher.Dispatcher, events *events.Emitter, mapHelper *modelaccesspolicy.Helper) *ThreadHandler {
	return &ThreadHandler{
		dispatcher: dispatcher,
		events:     events,
		mapHelper:  mapHelper,
	}
}

func convertTemplateThread(thread v1.Thread, share *v1.ThreadShare) types.ProjectTemplate {
	template := types.ProjectTemplate{
		Metadata:                         MetadataFrom(&thread),
		ProjectSnapshot:                  thread.Spec.Manifest,
		ProjectSnapshotStale:             thread.Status.UpgradeAvailable,
		ProjectSnapshotUpgradeInProgress: thread.Status.UpgradeInProgress || thread.Spec.UpgradeApproved,
		AssistantID:                      thread.Spec.AgentName,
		ProjectID:                        strings.Replace(thread.Spec.SourceThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		Ready:                            thread.Status.Created,
	}

	// Populate LastUpdated from status field
	if !thread.Status.LastUpgraded.IsZero() {
		template.ProjectSnapshotLastUpgraded = types.NewTime(thread.Status.LastUpgraded.Time)
	}

	if share != nil {
		template.PublicID = share.Spec.PublicID
		template.MCPServers = share.Status.MCPServers
	}

	template.Type = "projecttemplate"

	return template
}

func convertThread(thread v1.Thread) types.Thread {
	var (
		state = string(thread.Status.LastRunState)
	)
	if thread.Status.WorkflowState != "" {
		state = string(thread.Status.WorkflowState)
	}
	var env []string
	for _, e := range thread.Spec.Env {
		if e.Existing && e.Value == "" {
			env = append(env, e.Name)
		} else {
			env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
		}
	}

	return types.Thread{
		Metadata:       MetadataFrom(&thread),
		ThreadManifest: thread.Spec.Manifest,
		AssistantID:    thread.Spec.AgentName,
		TaskID:         thread.Spec.WorkflowName,
		TaskRunID:      thread.Spec.WorkflowExecutionName,
		LastRunID:      thread.Status.LastRunName,
		CurrentRunID:   thread.Status.CurrentRunName,
		State:          state,
		ProjectID:      strings.Replace(thread.Spec.ParentThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		UserID:         thread.Spec.UserID,
		Abort:          thread.Spec.Abort,
		SystemTask:     thread.Spec.SystemTask,
		Ephemeral:      thread.Spec.Ephemeral,
		Project:        thread.Spec.Project,
		Env:            env,
		Ready:          thread.Status.Created,
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
	// Don't allow update of allowed MCP tools here, do it with the mcpservers/{mcp_server_id}/tools endpoint
	newThread.AllowedMCPTools = existing.Spec.Manifest.AllowedMCPTools

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
		if !thread.DeletionTimestamp.IsZero() {
			continue
		}
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
	return getKnowledgeFile(req, &thread, nil, req.PathValue("file"))
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

	return uploadKnowledgeToWorkspace(req, a.dispatcher, ws, "", thread.Name, thread.Status.SharedKnowledgeSetName)
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

func (a *ThreadHandler) GetDefaultModelForThread(req api.Context) error {
	var thread v1.Thread
	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return fmt.Errorf("failed to get thread with id %s: %w", req.PathValue("thread_id"), err)
	}

	// We wipe out the model spec on the thread so that it tries to fetch the default model instead.
	thread.Spec.Manifest.Model = ""
	thread.Spec.Manifest.ModelProvider = ""

	model, modelProvider, err := threadmodel.GetModelAndModelProviderForThread(req.Context(), req.Storage, &thread)
	if err != nil {
		return fmt.Errorf("failed to get model and model provider for thread %s: %w", req.PathValue("thread_id"), err)
	}

	if model == string(types.DefaultModelAliasTypeLLM) {
		var alias v1.DefaultModelAlias
		if err := req.Get(&alias, string(types.DefaultModelAliasTypeLLM)); apierrors.IsNotFound(err) {
			// If the default model alias is not found, then nothing is configured, and we should just return nothing.
			return req.Write(map[string]string{
				"model":         "",
				"modelProvider": "",
			})
		} else if err != nil {
			return fmt.Errorf("failed to get default model alias for thread %s: %w", req.PathValue("thread_id"), err)
		}

		// This model has the system.ModelPrefix on it, so we set it and then let the next if statement take care of it.
		model = alias.Spec.Manifest.Model
	}

	if system.IsModelID(model) {
		// Check if the user has permission to use this model
		hasAccess, err := a.mapHelper.UserHasAccessToModel(req.User, model)
		if err != nil {
			return fmt.Errorf("failed to check model permission: %w", err)
		}
		if !hasAccess {
			// User doesn't have permission, return empty model
			return req.Write(map[string]string{
				"model":         "",
				"modelProvider": "",
			})
		}

		var modelObj v1.Model
		if err := req.Get(&modelObj, model); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to get model with id %s: %w", model, err)
		}

		// If we didn't find the model, the name and modelProvider will be empty
		model = modelObj.Spec.Manifest.Name
		modelProvider = modelObj.Spec.Manifest.ModelProvider
	}

	return req.Write(map[string]string{
		"model":         model,
		"modelProvider": modelProvider,
	})
}

func kickThread(ctx context.Context, c kclient.Client, namespace string, name string) error {
	var (
		thread v1.Thread
		key    = kclient.ObjectKey{Namespace: namespace, Name: name}
	)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := c.Get(ctx, key, &thread); err != nil {
			return kclient.IgnoreNotFound(err)
		}

		if thread.Annotations[v1.ThreadSyncAnnotation] != "" {
			delete(thread.Annotations, v1.ThreadSyncAnnotation)
		} else {
			if thread.Annotations == nil {
				thread.Annotations = make(map[string]string)
			}
			thread.Annotations[v1.ThreadSyncAnnotation] = "true"
		}

		return kclient.IgnoreNotFound(c.Update(ctx, &thread))
	})
}
