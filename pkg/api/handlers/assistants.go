package handlers

import (
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type AssistantHandler struct {
	invoker      *invoke.Invoker
	events       *events.Emitter
	dispatcher   *dispatcher.Dispatcher
	cachedClient kclient.WithWatch
}

func NewAssistantHandler(dispatcher *dispatcher.Dispatcher, invoker *invoke.Invoker, events *events.Emitter, cachedClient kclient.WithWatch) *AssistantHandler {
	return &AssistantHandler{
		invoker:      invoker,
		events:       events,
		dispatcher:   dispatcher,
		cachedClient: cachedClient,
	}
}

func getAssistant(req api.Context, id string) (*v1.Agent, error) {
	var agent v1.Agent
	if err := alias.Get(req.Context(), req.Storage, &agent, req.Namespace(), id); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (a *AssistantHandler) Abort(req api.Context) error {
	var (
		thread v1.Thread
	)

	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return err
	}

	return abortThread(req, &thread)
}

func abortThread(req api.Context, thread *v1.Thread) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := req.Storage.Get(req.Context(), kclient.ObjectKeyFromObject(thread), thread); err != nil {
			return err
		}
		if !thread.Spec.Abort {
			thread.Spec.Abort = true
			if err := req.Update(thread); err != nil {
				return err
			}
		}
		return nil
	})
}

func (a *AssistantHandler) Invoke(req api.Context) error {
	var (
		thread v1.Thread
	)

	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return err
	}

	input, err := req.Body()
	if err != nil {
		return err
	}

	resp, err := a.invoker.Thread(req.Context(), a.cachedClient, &thread, string(input), invoke.Options{
		GenerateName:    system.ChatRunPrefix,
		UserUID:         req.User.GetUID(),
		IgnoreMCPErrors: true,
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteCreated(map[string]string{
		"threadID": resp.Thread.Name,
		"runID":    resp.Run.Name,
		"message":  resp.Message,
	})
}

func (a *AssistantHandler) Get(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	agent, err := getAssistant(req, id)
	if err != nil {
		return err
	}

	return req.Write(convertAssistant(*agent))
}

func (a *AssistantHandler) List(req api.Context) error {
	var allAgents v1.AgentList
	if err := a.cachedClient.List(req.Context(), &allAgents, kclient.InNamespace(req.Namespace())); err != nil {
		return err
	}

	var result types.AssistantList
	for _, agent := range allAgents.Items {
		if agent.Spec.Manifest.Default || req.UserIsAdmin() {
			result.Items = append(result.Items, convertAssistant(agent))
		}
	}

	return req.Write(result)
}

func convertAssistant(agent v1.Agent) types.Assistant {
	var icons types.AgentIcons
	if agent.Spec.Manifest.Icons != nil {
		icons = *agent.Spec.Manifest.Icons
	}
	assistant := types.Assistant{
		Metadata:              MetadataFrom(&agent),
		Name:                  agent.Spec.Manifest.Name,
		Default:               agent.Spec.Manifest.Default,
		Description:           agent.Spec.Manifest.Description,
		EntityID:              agent.Name,
		StarterMessages:       agent.Spec.Manifest.StarterMessages,
		IntroductionMessage:   agent.Spec.Manifest.IntroductionMessage,
		Icons:                 icons,
		WebsiteKnowledge:      agent.Spec.Manifest.WebsiteKnowledge,
		AllowedModelProviders: agent.Spec.Manifest.AllowedModelProviders,
		AvailableThreadTools:  agent.Spec.Manifest.AvailableThreadTools,
		DefaultThreadTools:    agent.Spec.Manifest.DefaultThreadTools,
		Tools:                 agent.Spec.Manifest.Tools,
		AllowedModels:         agent.Spec.Manifest.AllowedModels,
	}
	if agent.Spec.Manifest.MaxThreadTools == 0 {
		assistant.MaxTools = DefaultMaxUserThreadTools
	} else {
		assistant.MaxTools = agent.Spec.Manifest.MaxThreadTools
	}
	if agent.Status.AliasAssigned {
		assistant.Alias = agent.Spec.Manifest.Alias
	}
	return assistant
}

func getProjectThread(req api.Context) (*v1.Thread, error) {
	var projectID = req.PathValue("project_id")
	if projectID == "" {
		return nil, types.NewErrNotFound("missing project id %s")
	}
	var thread v1.Thread
	if err := req.Get(&thread, strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)); apierrors.IsNotFound(err) {
		return nil, types.NewErrNotFound("project %s not found", projectID)
	} else if err != nil {
		return nil, err
	}
	return &thread, nil
}

func (a *AssistantHandler) DeleteCredential(req api.Context) error {
	var (
		cred = req.PathValue("cred_id")
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.GPTClient.DeleteCredential(req.Context(), thread.Name, cred); err != nil {
		return err
	}

	return nil
}

func (a *AssistantHandler) ListCredentials(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: []string{thread.Name},
	})
	if err != nil {
		return err
	}

	var result types.CredentialList
	for _, cred := range creds {
		result.Items = append(result.Items, convertCredential(cred))
	}

	return req.Write(result)
}

func (a *AssistantHandler) Events(req api.Context) error {
	var (
		follow  = req.URL.Query().Get("follow") == "true"
		history = req.URL.Query().Get("history") == "true"
		runID   = req.URL.Query().Get("runID")
		thread  v1.Thread
	)

	if runID == "" {
		runID = req.Request.Header.Get("Last-Event-ID")
	}

	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return err
	}

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		Follow:        follow,
		History:       history,
		LastRunName:   strings.TrimSuffix(runID, ":after"),
		MaxRuns:       10,
		After:         strings.HasSuffix(runID, ":after"),
		ThreadName:    thread.Name,
		WaitForThread: true,
	})
	if err != nil {
		return err
	}

	return req.WriteEvents(events)
}

func (a *AssistantHandler) SetEnv(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var envs map[string]string
	if err := req.Read(&envs); err != nil {
		return err
	}

	if err := setEnvMap(req, thread.Name, thread.Name, envs); err != nil {
		return err
	}

	var envVars []types.EnvVar
	for _, k := range slices.Sorted(maps.Keys(envs)) {
		envVars = append(envVars, types.EnvVar{
			Name:     k,
			Existing: true,
		})
	}
	thread.Spec.Env = envVars
	if err := req.Update(thread); err != nil {
		return err
	}

	return req.Write(envs)
}

func (a *AssistantHandler) GetEnv(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	data, err := getEnvMap(req, req.GPTClient, thread.Name, thread.Name)
	if err != nil {
		return err
	}

	return req.Write(data)
}

func (a *AssistantHandler) Knowledge(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if len(thread.Status.KnowledgeSetNames) == 0 {
		return req.Write(types.KnowledgeFileList{Items: []types.KnowledgeFile{}})
	}

	return listKnowledgeFiles(req, "", thread.Name, thread.Status.KnowledgeSetNames[0], nil)
}

func (a *AssistantHandler) GetKnowledgeFile(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}
	return getKnowledgeFile(req, thread, nil, req.PathValue("file"))
}

func (a *AssistantHandler) UploadKnowledge(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if len(thread.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHTTP(http.StatusTooEarly, "knowledge set is not available yet")
	}

	ws, err := getWorkspaceFromKnowledgeSet(req, thread.Status.KnowledgeSetNames[0])
	if err != nil {
		return err
	}

	return uploadKnowledgeToWorkspace(req, a.dispatcher, ws, "", thread.Name, thread.Status.KnowledgeSetNames[0])
}

func (a *AssistantHandler) DeleteKnowledge(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if len(thread.Status.KnowledgeSetNames) == 0 {
		return types.NewErrHTTP(http.StatusTooEarly, "knowledge set is not created yet")
	}

	return deleteKnowledge(req, req.PathValue("file"), thread.Status.KnowledgeSetNames[0])
}

func appendTools(result *types.AssistantToolList, added map[string]bool, toolsByName map[string]v1.ToolShortDescription, enabled, builtin bool, toolNames []string) {
	for _, toolName := range toolNames {
		if _, ok := added[toolName]; ok {
			continue
		}

		tool, ok := toolsByName[toolName]
		if !ok {
			continue
		}

		newTool := types.AssistantTool{
			Metadata: types.Metadata{
				ID: toolName,
			},
			ToolManifest: types.ToolManifest{
				Name:        tool.Name,
				Description: tool.Description,
				Icon:        tool.Metadata["icon"],
			},
			Enabled: enabled,
			Builtin: builtin,
		}

		added[toolName] = true
		result.Items = append(result.Items, newTool)
	}
}

func (a *AssistantHandler) RemoveTool(req api.Context) error {
	var (
		tool = req.PathValue("tool")
	)

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	removed := slices.DeleteFunc(thread.Spec.Manifest.Tools, func(s string) bool {
		return s == tool || s == ""
	})
	if len(removed) == len(thread.Spec.Manifest.Tools) {
		return types.NewErrNotFound("tool %s not found", tool)
	}
	thread.Spec.Manifest.Tools = removed
	if err := req.Update(thread); err != nil {
		return err
	}

	return a.Tools(req)
}

func (a *AssistantHandler) SetTools(req api.Context) error {
	var (
		tools    types.AssistantToolList
		agent    v1.Agent
		toolList []string
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return err
	}

	if err := req.Read(&tools); err != nil {
		return err
	}

	for _, tool := range tools.Items {
		if tool.Enabled && !tool.Builtin && !strings.HasPrefix(tool.ID, system.ToolPrefix) {
			toolList = append(toolList, tool.ID)
		}
	}

	if slices.Equal(thread.Spec.Manifest.Tools, toolList) {
		return a.Tools(req)
	}

	for _, tool := range toolList {
		if !slices.Contains(agent.Spec.Manifest.DefaultThreadTools, tool) && !slices.Contains(agent.Spec.Manifest.AvailableThreadTools, tool) {
			return types.NewErrBadRequest("tool %s is not available for this agent", tool)
		}
	}

	maxThreadTools := DefaultMaxUserThreadTools
	if agent.Spec.Manifest.MaxThreadTools > 0 {
		maxThreadTools = agent.Spec.Manifest.MaxThreadTools
	}

	if len(toolList) > maxThreadTools {
		return types.NewErrBadRequest("too many tools for this agent")
	}

	toolList = slices.DeleteFunc(toolList, func(s string) bool {
		return slices.Contains(agent.Spec.Manifest.Tools, s)
	})

	if thread.Spec.ParentThreadName != "" {
		var parentThread v1.Thread
		if err := req.Get(&parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		builtinTools, err := projects.GetStrings(req.Context(), req.Storage, &parentThread, func(t *v1.Thread) []string {
			return t.Spec.Manifest.Tools
		})
		if err != nil {
			return err
		}
		toolList = slices.DeleteFunc(toolList, func(s string) bool {
			return slices.Contains(builtinTools, s)
		})
	}

	thread.Spec.Manifest.Tools = toolList
	if err := req.Update(thread); err != nil {
		return err
	}

	return a.Tools(req)
}

func (a *AssistantHandler) Tools(req api.Context) error {
	var (
		id     = req.PathValue("assistant_id")
		thread v1.Thread
	)

	agent, err := getAssistant(req, id)
	if err != nil {
		return err
	}

	project, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if !project.Spec.Project {
		thread = *project
		var newProject v1.Thread
		if err := req.Get(&newProject, project.Spec.ParentThreadName); err != nil {
			return err
		}
		project = &newProject
	}

	var parentProject v1.Thread
	if project.Spec.ParentThreadName != "" {
		if err := req.Get(&parentProject, project.Spec.ParentThreadName); err != nil {
			return err
		}
	}

	enabledTool := make(map[string]bool)
	for _, tool := range thread.Spec.Manifest.Tools {
		enabledTool[tool] = true
	}
	for _, tool := range project.Spec.Manifest.Tools {
		enabledTool[tool] = true
	}
	for _, tool := range parentProject.Spec.Manifest.Tools {
		enabledTool[tool] = true
	}

	var tools v1.ToolReferenceList
	if err := req.List(&tools); err != nil {
		return err
	}

	toolsByName := make(map[string]v1.ToolShortDescription)
	for _, tool := range tools.Items {
		if tool.Status.Tool != nil {
			toolsByName[tool.Name] = *tool.Status.Tool
		}
	}

	var (
		added  = map[string]bool{}
		result = types.AssistantToolList{
			Items: []types.AssistantTool{},
		}
	)

	appendTools(&result, added, toolsByName, true, true, agent.Spec.Manifest.Tools)
	appendTools(&result, added, toolsByName, true, true, parentProject.Spec.Manifest.Tools)
	appendTools(&result, added, toolsByName, true, thread.Name != "", project.Spec.Manifest.Tools)
	appendTools(&result, added, toolsByName, true, false, thread.Spec.Manifest.Tools)
	appendTools(&result, added, toolsByName, false, false, agent.Spec.Manifest.DefaultThreadTools)
	appendTools(&result, added, toolsByName, false, false, agent.Spec.Manifest.AvailableThreadTools)

	var (
		userTools      v1.ToolList
		toolThreadName = project.Name
	)
	if project.Spec.ParentThreadName != "" {
		toolThreadName = project.Spec.ParentThreadName
	}
	if err := req.List(&userTools, kclient.MatchingFields{"spec.threadName": toolThreadName}); err != nil {
		return err
	}

	for _, tool := range userTools.Items {
		result.Items = append(result.Items, convertTool(tool, true))
	}

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].Name < result.Items[j].Name
	})

	return req.Write(result)
}
