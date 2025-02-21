package handlers

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AssistantHandler struct {
	invoker      *invoke.Invoker
	events       *events.Emitter
	gptScript    *gptscript.GPTScript
	cachedClient kclient.Client
}

func NewAssistantHandler(invoker *invoke.Invoker, events *events.Emitter, gptScript *gptscript.GPTScript, cachedClient kclient.Client) *AssistantHandler {
	return &AssistantHandler{
		invoker:      invoker,
		events:       events,
		gptScript:    gptScript,
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

	resp, err := a.invoker.Thread(req.Context(), req.Storage, &thread, string(input), invoke.Options{
		GenerateName: system.ChatRunPrefix,
		UserUID:      req.User.GetUID(),
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteCreated(map[string]string{
		"threadID": resp.Thread.Name,
		"runID":    resp.Run.Name,
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
	var (
		keys   = make([]string, 0, 3)
		result types.AssistantList
		user   = req.User
	)

	keys = append(keys, "*", user.GetUID())
	if attr := user.GetExtra()["email"]; len(attr) > 0 {
		keys = append(keys, attr...)
	}

	seen := map[string]struct{}{}
	for _, key := range keys {
		var access v1.AgentAuthorizationList
		err := a.cachedClient.List(req.Context(), &access, kclient.InNamespace(req.Namespace()), kclient.MatchingFields{
			"spec.userID": key,
		})
		if err != nil {
			return err
		}
		for _, auth := range access.Items {
			if _, ok := seen[auth.Spec.AgentID]; ok {
				continue
			}
			var agent v1.Agent
			if err := a.cachedClient.Get(req.Context(), router.Key(req.Namespace(), auth.Spec.AgentID), &agent); apierrors.IsNotFound(err) {
				continue
			} else if err != nil {
				return err
			}
			result.Items = append(result.Items, convertAssistant(agent))
			seen[auth.Spec.AgentID] = struct{}{}
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
		Metadata:            MetadataFrom(&agent),
		Name:                agent.Spec.Manifest.Name,
		Default:             agent.Spec.Manifest.Default,
		Description:         agent.Spec.Manifest.Description,
		EntityID:            agent.ObjectMeta.Name,
		StarterMessages:     agent.Spec.Manifest.StarterMessages,
		IntroductionMessage: agent.Spec.Manifest.IntroductionMessage,
		Icons:               icons,
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
		runID  = req.Request.Header.Get("Last-Event-ID")
		thread v1.Thread
	)

	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return err
	}

	_, events, err := a.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		Follow:        true,
		History:       runID == "",
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

	if err := setEnvMap(req, a.gptScript, thread.Name, thread.Name, envs); err != nil {
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

	data, err := getEnvMap(req, a.gptScript, thread.Name, thread.Name)
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
	return getKnowledgeFile(req, a.gptScript, thread, nil, req.PathValue("file"))
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

	return uploadKnowledgeToWorkspace(req, a.gptScript, ws, "", thread.Name, thread.Status.KnowledgeSetNames[0])
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

func (a *AssistantHandler) AddTool(req api.Context) (retErr error) {
	defer func() {
		if retErr == nil {
			retErr = a.Tools(req)
		}
	}()

	var (
		id           = req.PathValue("assistant_id")
		tool         = req.PathValue("tool")
		toolManifest types.ToolManifest
		hasBody      bool
	)

	//nolint:revive
	if err := req.Read(&toolManifest); errors.Is(err, io.EOF) {
	} else if err != nil {
		return err
	} else {
		// only set has body if the id is a tool id and there's a body
		hasBody = system.IsToolID(tool)
	}

	agent, err := getAssistant(req, id)
	if err != nil {
		return err
	}

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if slices.Contains(thread.Spec.Manifest.Tools, tool) && !hasBody {
		return nil
	}

	if system.IsToolID(tool) {
		var customTool v1.Tool
		if err := req.Get(&customTool, tool); err != nil {
			return err
		}
		if customTool.Spec.ThreadName == thread.Name {
			if hasBody {
				customTool.Spec.Manifest = toolManifest
				return req.Update(&customTool)
			}
			thread.Spec.Manifest.Tools = append(thread.Spec.Manifest.Tools, tool)
			return req.Update(thread)
		}
	}

	if !slices.Contains(agent.Spec.Manifest.AvailableThreadTools, tool) &&
		!slices.Contains(agent.Spec.Manifest.DefaultThreadTools, tool) {
		return types.NewErrBadRequest("tool %s is not available", tool)
	}

	maxTools := DefaultMaxUserThreadTools
	if agent.Spec.Manifest.MaxThreadTools > 0 {
		maxTools = agent.Spec.Manifest.MaxThreadTools
	}

	if len(thread.Spec.Manifest.Tools) >= maxTools {
		return types.NewErrBadRequest("maximum number of tools (%d) reached", maxTools)
	}

	thread.Spec.Manifest.Tools = append(thread.Spec.Manifest.Tools, tool)
	return req.Update(thread)
}

func (a *AssistantHandler) DeleteTool(req api.Context) error {
	var (
		toolID = req.PathValue("tool")
	)

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	var tool v1.Tool
	if err := req.Get(&tool, toolID); err != nil {
		return err
	}

	if tool.Spec.ThreadName != thread.Name {
		return types.NewErrNotFound("tool %s is not available", toolID)
	}

	if err := req.Delete(&tool); err != nil {
		return err
	}

	if slices.Contains(thread.Spec.Manifest.Tools, toolID) {
		thread.Spec.Manifest.Tools = slices.DeleteFunc(thread.Spec.Manifest.Tools, func(s string) bool {
			return s == toolID || s == ""
		})
		if err := req.Update(thread); err != nil {
			return err
		}
	}

	return nil
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
		if tool.Enabled && !tool.Builtin {
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

	thread.Spec.Manifest.Tools = toolList
	if err := req.Update(thread); err != nil {
		return err
	}

	return a.Tools(req)
}

func (a *AssistantHandler) Tools(req api.Context) error {
	var (
		id = req.PathValue("assistant_id")
	)

	agent, err := getAssistant(req, id)
	if err != nil {
		return err
	}

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	enabledTool := make(map[string]bool)
	for _, tool := range thread.Spec.Manifest.Tools {
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

	if thread.Name == "" {
		result.ReadOnly = true
		appendTools(&result, added, toolsByName, true, false, agent.Spec.Manifest.DefaultThreadTools)
	} else {
		appendTools(&result, added, toolsByName, true, false, thread.Spec.Manifest.Tools)
		appendTools(&result, added, toolsByName, false, false, agent.Spec.Manifest.DefaultThreadTools)
	}

	appendTools(&result, added, toolsByName, false, false, agent.Spec.Manifest.AvailableThreadTools)

	var userTools v1.ToolList
	if err := req.List(&userTools, kclient.MatchingFields{"spec.threadName": thread.Name}); err != nil {
		return err
	}

	for _, tool := range userTools.Items {
		result.Items = append(result.Items, convertTool(tool, slices.Contains(thread.Spec.Manifest.Tools, tool.Name)))
	}

	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].Name < result.Items[j].Name
	})

	return req.Write(result)
}
