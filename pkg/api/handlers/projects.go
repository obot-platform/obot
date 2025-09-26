package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	threadmodel "github.com/obot-platform/obot/pkg/thread"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectsHandler struct {
	cachedClient kclient.Client
	invoker      *invoke.Invoker
}

func NewProjectsHandler(cachedClient kclient.Client, invoker *invoke.Invoker) *ProjectsHandler {
	return &ProjectsHandler{
		cachedClient: cachedClient,
		invoker:      invoker,
	}
}

func (h *ProjectsHandler) ListMembers(req api.Context) error {
	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	var threadAuths v1.ThreadAuthorizationList
	if err := req.List(&threadAuths, kclient.MatchingFields{
		"spec.threadID": thread.Name,
	}); err != nil {
		return err
	}

	result := make([]types.ProjectMember, 0, len(threadAuths.Items)+1)
	for _, threadAuth := range threadAuths.Items {
		user, err := req.GatewayClient.UserByID(req.Context(), threadAuth.Spec.UserID)
		if err != nil {
			return err
		}

		result = append(result, types.ProjectMember{
			UserID:  threadAuth.Spec.UserID,
			IconURL: user.IconURL,
			Email:   user.Email,
			IsOwner: false,
		})
	}

	// Also get the details of the project owner.
	owner, err := req.GatewayClient.UserByID(req.Context(), thread.Spec.UserID)
	if err != nil {
		return err
	}

	result = append(result, types.ProjectMember{
		UserID:  thread.Spec.UserID,
		IconURL: owner.IconURL,
		Email:   owner.Email,
		IsOwner: true,
	})

	return req.Write(result)
}

func (h *ProjectsHandler) DeleteMember(req api.Context) error {
	memberID := req.PathValue("member_id")

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if !thread.Spec.Project {
		return types.NewErrBadRequest("only projects can have members")
	}

	if !req.UserIsAdmin() && thread.Spec.UserID != req.User.GetUID() {
		return types.NewErrBadRequest("only the project creator can remove members")
	}

	if memberID == thread.Spec.UserID {
		return types.NewErrBadRequest("cannot remove the project creator")
	}

	// Find the member's authorization
	var memberships v1.ThreadAuthorizationList
	if err := req.List(&memberships, kclient.MatchingFields{
		"spec.threadID": thread.Name,
		"spec.userID":   memberID,
	}); err != nil {
		return err
	}

	if len(memberships.Items) == 0 {
		return types.NewErrNotFound("user is not a member of this project")
	}

	// Delete all authorizations for this user and thread
	for _, membership := range memberships.Items {
		if err := req.Delete(&membership); err != nil {
			return err
		}
	}

	return nil
}

func (h *ProjectsHandler) UpdateProject(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		project   types.ProjectManifest
	)

	if err := req.Read(&project); err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)); err != nil {
		return err
	}

	agent, err := getAssistant(req, thread.Spec.AgentName)
	if err != nil {
		return err
	}

	project.Tools = thread.Spec.Manifest.Tools
	project.AllowedMCPTools = thread.Spec.Manifest.AllowedMCPTools

	if !equality.Semantic.DeepEqual(thread.Spec.Manifest, project) {
		// Make sure that the default model provider and model are also on the models map.
		if project.DefaultModelProvider != "" {
			if project.Models == nil {
				project.Models = map[string][]string{}
			}

			if !slices.Contains(project.Models[project.DefaultModelProvider], project.DefaultModel) {
				project.Models[project.DefaultModelProvider] = append(project.Models[project.DefaultModelProvider], project.DefaultModel)
			}
		}

		// Make sure that all the specified model providers are allowed.
		for provider := range project.Models {
			if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, provider) {
				return types.NewErrBadRequest("model provider %s is not allowed for agent %s", provider, agent.Name)
			}
		}

		if project.Capabilities != nil {
			thread.Spec.Capabilities = v1.ThreadCapabilities(*project.Capabilities)
		}
		thread.Spec.Manifest = project.ThreadManifest
		thread.Spec.DefaultModelProvider = project.DefaultModelProvider
		thread.Spec.DefaultModel = project.DefaultModel
		thread.Spec.Models = project.Models
		if err := req.Update(&thread); err != nil {
			return err
		}
	}

	return req.Write(convertProject(&thread, nil))
}

func (h *ProjectsHandler) CopyProject(req api.Context) error {
	var (
		thread     v1.Thread
		projectID  = req.PathValue("project_id")
		threadName = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)
	if err := req.Get(&thread, threadName); err != nil {
		return err
	}

	for thread.Spec.ParentThreadName != "" {
		if err := req.Get(&thread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
	}

	if !thread.Spec.Project {
		return types.NewErrBadRequest("invalid project %s", projectID)
	}

	newThread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ThreadSpec{
			Manifest:         thread.Spec.Manifest,
			AgentName:        thread.Spec.AgentName,
			SourceThreadName: thread.Name,
			Project:          true,
			UserID:           req.User.GetUID(),
			// Explicit ignoring model provider and model here. The user will have to provide their own credentials.
		},
	}

	if newThread.Spec.Manifest.Name != "" {
		newThread.Spec.Manifest.Name = "Copy of " + newThread.Spec.Manifest.Name
	} else {
		newThread.Spec.Manifest.Name = "Copy"
	}

	if err := req.Create(&newThread); err != nil {
		return err
	}

	return req.Write(convertProject(&newThread, nil))
}

func (h *ProjectsHandler) GetProject(req api.Context) error {
	var (
		thread    v1.Thread
		projectID = strings.Replace(req.PathValue("project_id"), system.ProjectPrefix, system.ThreadPrefix, 1)
	)
	if err := req.Get(&thread, projectID); err != nil {
		return err
	}

	if thread.Spec.Template {
		return types.NewErrBadRequest("template projects are not supported")
	}

	var parentThread v1.Thread
	if thread.Spec.ParentThreadName != "" {
		if err := req.Get(&parentThread, thread.Spec.ParentThreadName); err == nil {
			return req.Write(convertProject(&thread, &parentThread))
		}
	}
	return req.Write(convertProject(&thread, nil))
}

// UpgradeFromTemplate upgrades a project to the latest snapshot of a project if the snapshot has changed since
// the project was created or the last time the project was upgraded.
func (h *ProjectsHandler) UpgradeFromTemplate(req api.Context) error {
	var (
		projectID = strings.Replace(req.PathValue("project_id"), system.ProjectPrefix, system.ThreadPrefix, 1)
		thread    v1.Thread
	)

	if err := req.Get(&thread, projectID); err != nil {
		return err
	}

	if thread.Spec.SourceThreadName == "" || !thread.Spec.Project {
		return types.NewErrBadRequest("project was not created from a template")
	}

	if thread.Status.UpgradeInProgress {
		return types.NewErrHTTP(http.StatusTooEarly, "project upgrade already in progress")
	}

	if !thread.Status.UpgradeAvailable {
		// Project is ineligable for an upgrade due to one of the following reasons:
		// - the project is already at the latest revision of the project snapshot
		// - the user has manually modified the project
		return types.NewErrBadRequest("project not eligible for an upgrade")
	}

	if thread.Spec.UpgradeApproved {
		// Project is already approved for an upgrade, nothing to do
		return nil
	}

	// Get the source thread to verify it's a template
	var source v1.Thread
	if err := req.Get(&source, thread.Spec.SourceThreadName); err != nil {
		return err
	}

	// Verify the source is actually a template
	if !source.Spec.Template {
		return types.NewErrBadRequest("source project is not a template")
	}

	// Ensure the template isn't currently being upgraded from its own source project
	if source.Status.UpgradeInProgress {
		return types.NewErrHTTP(http.StatusTooEarly, "the project snapshot is currently being upgraded")
	}

	// Project has diverged from the snapshot, upgrade to the latest snapshot by setting the upgrade flag
	thread.Spec.UpgradeApproved = true

	return req.Update(&thread)
}

func (h *ProjectsHandler) ListProjects(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		hasEditor   = req.URL.Query().Has("editor")
		isEditor    = req.URL.Query().Get("editor") == "true"

		agent *v1.Agent
		err   error
	)

	if assistantID != "" {
		agent, err = getAssistant(req, assistantID)
		if err != nil {
			return err
		}
	}

	projects, err := h.getProjects(req, agent, (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true")
	if err != nil {
		return err
	}

	if hasEditor {
		projects.Items = filterEditorProjects(projects.Items, isEditor)
	}

	return req.Write(projects)
}

func filterEditorProjects(projects []types.Project, isEditor bool) []types.Project {
	result := make([]types.Project, 0, len(projects))
	for _, project := range projects {
		if project.Editor == isEditor {
			result = append(result, project)
		}
	}
	return result
}

func (h *ProjectsHandler) getProjectThread(req api.Context) (*v1.Thread, error) {
	var (
		thread    v1.Thread
		projectID = strings.Replace(req.PathValue("project_id"), system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	if projectID == "default" {
		return getThreadForScope(req)
	}

	return &thread, req.Get(&thread, projectID)
}

func (h *ProjectsHandler) DeleteProject(req api.Context) error {
	project, err := h.getProjectThread(req)
	if err != nil {
		return err
	}

	if !req.UserIsAdmin() && project.Spec.UserID != req.User.GetUID() {
		return types.NewErrBadRequest("only the project creator can delete this project")
	}

	return req.Delete(project)
}

func (h *ProjectsHandler) CreateProject(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		agent       *v1.Agent
		err         error
	)

	agent, err = getAssistant(req, assistantID)
	if err != nil {
		return err
	}

	var project types.ProjectManifest
	if err := req.Read(&project); err != nil {
		return err
	}

	if project.DefaultModelProvider != "" && !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, project.DefaultModelProvider) {
		return types.NewErrBadRequest("model provider %s is not allowed for agent %s", project.DefaultModelProvider, agent.Name)
	}

	for provider := range project.Models {
		if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, provider) {
			return types.NewErrBadRequest("model provider %s is not allowed for agent %s", provider, agent.Name)
		}
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    agent.Namespace,
			Finalizers:   []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			Manifest: types.ThreadManifest{
				Tools: agent.Spec.Manifest.DefaultThreadTools,
				ThreadManifestManagedFields: types.ThreadManifestManagedFields{
					Name:        project.Name,
					Description: project.Description,
				},
				Prompt: project.Prompt,
			},
			AgentName:            agent.Name,
			Project:              true,
			UserID:               req.User.GetUID(),
			DefaultModelProvider: project.DefaultModelProvider,
			DefaultModel:         project.DefaultModel,
			Models:               project.Models,
		},
	}
	if project.Capabilities != nil {
		thread.Spec.Capabilities = v1.ThreadCapabilities(*project.Capabilities)
	}

	if err := req.Create(thread); err != nil {
		return err
	}

	return req.WriteCreated(convertProject(thread, nil))
}

func (h *ProjectsHandler) getProjects(req api.Context, agent *v1.Agent, all bool) (result types.ProjectList, err error) {
	var (
		threads v1.ThreadList
		auths   v1.ThreadAuthorizationList
		seen    = make(map[string]bool)
		fields  = kclient.MatchingFields{
			"spec.project":  "true",
			"spec.template": "false",
		}
	)

	// If not all, filter for current user
	if !all {
		fields["spec.userUID"] = req.User.GetUID()
	}

	// Agent may be nil if
	if agent != nil {
		fields["spec.agentName"] = agent.Name
	}

	err = req.List(&threads, fields)
	if err != nil {
		return result, err
	}

	for _, thread := range threads.Items {
		seen[thread.Name] = true
		var parentThread v1.Thread
		if thread.Spec.ParentThreadName != "" {
			if err := req.Get(&parentThread, thread.Spec.ParentThreadName); err == nil {
				result.Items = append(result.Items, convertProject(&thread, &parentThread))
				continue
			}
		}
		result.Items = append(result.Items, convertProject(&thread, nil))
	}

	// Check if the user is a member of any other projects.
	if err = req.List(&auths, kclient.MatchingFields{
		"spec.userID": req.User.GetUID(),
	}); err != nil {
		return result, err
	}

	for _, auth := range auths.Items {
		if seen[auth.Spec.ThreadID] {
			continue
		}
		var thread v1.Thread
		if err := h.cachedClient.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: auth.Spec.ThreadID}, &thread); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return result, err
		}

		if !thread.Spec.Project {
			continue
		}

		if agent != nil && thread.Spec.AgentName != agent.Name {
			continue
		}

		var parentThread v1.Thread
		if thread.Spec.ParentThreadName != "" {
			if err := req.Get(&parentThread, thread.Spec.ParentThreadName); err == nil {
				result.Items = append(result.Items, convertProject(&thread, &parentThread))
				seen[auth.Spec.ThreadID] = true
				continue
			}
		}
		result.Items = append(result.Items, convertProject(&thread, nil))
		seen[auth.Spec.ThreadID] = true
	}

	return result, nil
}

func convertProject(thread *v1.Thread, parentThread *v1.Thread) types.Project {
	p := types.Project{
		Metadata: MetadataFrom(thread),
		ProjectManifest: types.ProjectManifest{
			ThreadManifest:       thread.Spec.Manifest,
			DefaultModelProvider: thread.Spec.DefaultModelProvider,
			DefaultModel:         thread.Spec.DefaultModel,
			Models:               thread.Spec.Models,
			Capabilities:         convertProjectCapabilities(thread.Spec.Capabilities),
		},
		ParentID:                     strings.Replace(thread.Spec.ParentThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		SourceProjectID:              strings.Replace(thread.Spec.SourceThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		AssistantID:                  thread.Spec.AgentName,
		Editor:                       thread.IsEditor(),
		UserID:                       thread.Spec.UserID,
		WorkflowNamesFromIntegration: thread.Status.WorkflowNamesFromIntegration,
		TemplateUpgradeAvailable:     (thread.Status.UpgradeAvailable && !thread.Spec.UpgradeApproved),
		TemplateUpgradeInProgress:    thread.Status.UpgradeInProgress,
		TemplatePublicID:             thread.Status.UpgradePublicID,
	}

	if !thread.Status.LastUpgraded.IsZero() {
		p.TemplateLastUpgraded = types.NewTime(thread.Status.LastUpgraded.Time)
	}

	// Include tools from parent project
	if parentThread != nil {
		p.Tools = append(p.Tools, parentThread.Spec.Manifest.Tools...)
	}

	p.Type = "project"
	p.ID = strings.Replace(p.ID, system.ThreadPrefix, system.ProjectPrefix, 1)
	return p
}

func convertProjectCapabilities(capabilities v1.ThreadCapabilities) *types.ProjectCapabilities {
	result := types.ProjectCapabilities{
		OnSlackMessage:   capabilities.OnSlackMessage,
		OnDiscordMessage: capabilities.OnDiscordMessage,
		OnEmail:          capabilities.OnEmail,
	}
	if capabilities.OnWebhook != nil {
		result.OnWebhook = &types.OnWebhook{}
		result.OnWebhook.ValidationHeader = capabilities.OnWebhook.ValidationHeader
		result.OnWebhook.Headers = capabilities.OnWebhook.Headers
		result.OnWebhook.Secret = "********"
	}
	return &result
}

func (h *ProjectsHandler) DeleteProjectThread(req api.Context) error {
	var thread v1.Thread
	if err := req.Get(&thread, req.PathValue("thread_id")); err != nil {
		return err
	}
	return req.Delete(&thread)
}

func (h *ProjectsHandler) CreateProjectThread(req api.Context) error {
	projectThread, err := h.getProjectThread(req)
	if err != nil {
		return err
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadPrefix,
			Namespace:    projectThread.Namespace,
			Finalizers:   []string{v1.ThreadFinalizer},
		},
		Spec: v1.ThreadSpec{
			AgentName:        projectThread.Spec.AgentName,
			ParentThreadName: projectThread.Name,
			UserID:           req.User.GetUID(),
		},
	}

	body, err := io.ReadAll(req.Request.Body)
	if err != nil {
		return err
	}

	if len(body) > 0 {
		// Attempt to parse the model and model provider from the body.
		var bodyContents struct {
			Model         string `json:"model"`
			ModelProvider string `json:"modelProvider"`
		}

		if err := json.Unmarshal(body, &bodyContents); err != nil {
			return fmt.Errorf("failed to unmarshal request body: %w", err)
		}

		// Make sure that this model and model provider are valid.
		if bodyContents.Model != "" || bodyContents.ModelProvider != "" {
			agent, err := getAssistant(req, projectThread.Spec.AgentName)
			if err != nil {
				return err
			}

			// Check if model is allowed by assistant OR project
			allowedByAssistant := len(agent.Spec.Manifest.AllowedModels) == 0 || slices.Contains(agent.Spec.Manifest.AllowedModels, bodyContents.Model)
			allowedByProject := false
			if projectModels, ok := projectThread.Spec.Models[bodyContents.ModelProvider]; ok {
				allowedByProject = slices.Contains(projectModels, bodyContents.Model)
			}

			// if bodyContents.ModelProvider is empty it means that it is set at global level so allowedByProject should be true
			if bodyContents.ModelProvider != "" {
				allowedByProject = true
			}

			if !allowedByAssistant && !allowedByProject {
				return types.NewErrBadRequest("model %q is not allowed for assistant and project", bodyContents.Model)
			}
		}

		thread.Spec.Manifest.Model = bodyContents.Model
		thread.Spec.Manifest.ModelProvider = bodyContents.ModelProvider
	}

	if err := req.Create(&thread); err != nil {
		return err
	}

	return req.WriteCreated(convertThread(thread))
}

func (h *ProjectsHandler) GetProjectThread(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	var thread v1.Thread
	if err := req.Get(&thread, id); err != nil {
		return err
	}

	return req.Write(convertThread(thread))
}

func (h *ProjectsHandler) streamThreads(req api.Context, matches func(t *v1.Thread) bool, opts ...kclient.ListOption) error {
	c, err := api.Watch[*v1.Thread](req, &v1.ThreadList{}, opts...)
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("Content-Type", "text/event-stream")
	for thread := range c {
		if !matches(thread) {
			continue
		}
		if err := req.WriteDataEvent(convertThread(*thread)); err != nil {
			return err
		}
	}

	return nil
}

func (h *ProjectsHandler) ListProjectThreads(req api.Context) error {
	var (
		threads v1.ThreadList
	)

	projectThread, err := h.getProjectThread(req)
	if err != nil {
		return err
	}

	if req.IsStreamRequested() {
		// Field selectors don't work right now....
		return h.streamThreads(req, func(t *v1.Thread) bool {
			return !t.Spec.Project &&
				!t.Spec.Ephemeral &&
				t.Spec.ParentThreadName == projectThread.Name &&
				t.Spec.UserID == req.User.GetUID()
		})
	}

	selector := kclient.MatchingFields{
		"spec.project":          "false",
		"spec.parentThreadName": projectThread.Name,
	}

	if err := req.List(&threads, selector); err != nil {
		return err
	}

	var result types.ThreadList
	for _, thread := range threads.Items {
		if !thread.DeletionTimestamp.IsZero() {
			continue
		}
		if thread.Spec.Ephemeral {
			continue
		}
		result.Items = append(result.Items, convertThread(thread))
	}

	return req.Write(result)
}

func (h *ProjectsHandler) ListLocalCredentials(req api.Context) error {
	return h.listCredentials(req, true)
}

func (h *ProjectsHandler) ListCredentials(req api.Context) error {
	return h.listCredentials(req, false)
}

func (h *ProjectsHandler) listCredentials(req api.Context, local bool) error {
	var (
		tools               = make(map[string]struct{})
		existingCredentials = make(map[string]string)
		result              types.ProjectCredentialList
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var agent v1.Agent
	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return err
	}

	allTools := slices.Concat(agent.Spec.Manifest.Tools,
		agent.Spec.Manifest.DefaultThreadTools,
		agent.Spec.Manifest.AvailableThreadTools,
		thread.Spec.Manifest.Tools)
	for _, tool := range allTools {
		tools[tool] = struct{}{}
	}

	credContextID := thread.Name
	if local {
		credContextID = thread.Name + "-local"
	}

	credContexts := []string{credContextID}
	if thread.Spec.ParentThreadName != "" && !local {
		credContexts = append(credContexts, thread.Spec.ParentThreadName)
	}
	credContexts = append(credContexts, thread.Spec.AgentName)

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credContexts,
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		existingCredentials[cred.ToolName] = cred.Context
	}

	for tool := range tools {
		var toolRef v1.ToolReference
		if err := req.Get(&toolRef, tool); apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}

		if toolRef.Status.Tool == nil || len(toolRef.Status.Tool.CredentialNames) == 0 {
			continue
		}

		exists := true
		baseAgentCred := false
		for _, credName := range toolRef.Status.Tool.CredentialNames {
			credCtx, ok := existingCredentials[credName]
			baseAgentCred = credCtx == thread.Spec.AgentName
			if !ok {
				exists = false
				break
			}
		}

		result.Items = append(result.Items, types.ProjectCredential{
			ToolID:    toolRef.Name,
			ToolName:  toolRef.Status.Tool.Name,
			Icon:      toolRef.Status.Tool.Metadata["icon"],
			Exists:    exists,
			BaseAgent: baseAgentCred,
		})
	}

	return req.Write(result)
}

func (h *ProjectsHandler) LocalAuthenticate(req api.Context) (err error) {
	return h.authenticate(req, true)
}

func (h *ProjectsHandler) Authenticate(req api.Context) (err error) {
	return h.authenticate(req, false)
}

func (h *ProjectsHandler) authenticate(req api.Context, local bool) (err error) {
	var (
		agent v1.Agent
		tools = strings.Split(req.PathValue("tools"), ",")
	)

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for authentication")
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return err
	}

	credContext := thread.Name
	if local {
		credContext = thread.Name + "-local"
	}
	resp, err := runAuthForAgent(req, h.invoker, &agent, credContext, tools, req.User.GetUID(), thread.Name)
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (h *ProjectsHandler) LocalDeAuthenticate(req api.Context) error {
	return h.deAuthenticate(req, true)
}

func (h *ProjectsHandler) DeAuthenticate(req api.Context) error {
	return h.deAuthenticate(req, false)
}

func (h *ProjectsHandler) deAuthenticate(req api.Context, local bool) error {
	var (
		agent v1.Agent
		tools = strings.Split(req.PathValue("tools"), ",")
	)

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for de-authentication")
	}

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return err
	}

	credContext := thread.Name
	if local {
		credContext = thread.Name + "-local"
	}

	errs := removeToolCredentials(req, credContext, agent.Namespace, tools)
	return errors.Join(errs...)
}

func (h *ProjectsHandler) GetDefaultModelForProject(req api.Context) error {
	var project v1.Thread
	projectID := req.PathValue("project_id")
	threadID := strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)

	if err := req.Get(&project, threadID); err != nil {
		return fmt.Errorf("failed to get project with id %s: %w", projectID, err)
	}

	if !project.Spec.Project {
		return types.NewErrBadRequest("thread %s is not a project", threadID)
	}

	model, modelProvider, err := threadmodel.GetModelAndModelProviderForProject(req.Context(), req.Storage, &project)
	if err != nil {
		return fmt.Errorf("failed to get model and model provider for project %s: %w", projectID, err)
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
			return fmt.Errorf("failed to get default model alias for project %s: %w", projectID, err)
		}

		// This model has the system.ModelPrefix on it, so we set it and then let the next if statement take care of it.
		model = alias.Spec.Manifest.Model
	}

	if strings.HasPrefix(model, system.ModelPrefix) {
		var modelObj v1.Model
		if err := req.Get(&modelObj, model); err != nil {
			return fmt.Errorf("failed to get model with id %s: %w", model, err)
		}

		model = modelObj.Spec.Manifest.Name
		modelProvider = modelObj.Spec.Manifest.ModelProvider
	}

	return req.Write(map[string]string{
		"model":         model,
		"modelProvider": modelProvider,
	})
}
