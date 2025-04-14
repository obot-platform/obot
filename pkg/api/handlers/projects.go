package handlers

import (
	"errors"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectsHandler struct {
	cachedClient kclient.Client
	invoker      *invoke.Invoker
	gptScript    *gptscript.GPTScript
}

func NewProjectsHandler(cachedClient kclient.Client, invoker *invoke.Invoker, gptScript *gptscript.GPTScript) *ProjectsHandler {
	return &ProjectsHandler{
		cachedClient: cachedClient,
		invoker:      invoker,
		gptScript:    gptScript,
	}
}

func (h *ProjectsHandler) UpdateAuthorizations(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		threadID  = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
		auths     types.ProjectAuthorizationList
		existing  = map[string]struct{}{}
	)

	if err := req.Read(&auths); err != nil {
		return err
	}

	var threadAuths v1.ThreadAuthorizationList
	if err := req.List(&threadAuths, kclient.MatchingFields{
		"spec.threadID": threadID,
	}); err != nil {
		return err
	}

	for _, threadAuth := range threadAuths.Items {
		existing[threadAuth.Spec.UserID] = struct{}{}
	}

	for _, auth := range auths.Items {
		if strings.TrimSpace(auth.Target) == "" {
			continue
		}
		if _, ok := existing[auth.Target]; !ok {
			err := req.Create(&v1.ThreadAuthorization{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.ThreadAuthorizationPrefix,
					Namespace:    req.Namespace(),
				},
				Spec: v1.ThreadAuthorizationSpec{
					ThreadAuthorizationManifest: types.ThreadAuthorizationManifest{
						ThreadID: threadID,
						UserID:   auth.Target,
					},
				},
			})
			if err != nil {
				return err
			}
		} else {
			delete(existing, auth.Target)
		}
	}

	for target := range existing {
		for _, threadAuth := range threadAuths.Items {
			if threadAuth.Spec.UserID == target {
				if err := req.Delete(&threadAuth); err != nil {
					return err
				}
			}
		}
	}

	return h.ListAuthorizations(req)
}

func (h *ProjectsHandler) RejectPendingAuthorization(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		threadID  = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
		auths     v1.ThreadAuthorizationList
		thread    v1.Thread
	)

	email, ok := getEmail(req)
	if !ok {
		return types.NewErrBadRequest("email is required")
	}

	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	err := req.List(&auths, kclient.MatchingFields{
		"spec.threadID": threadID,
		"spec.userID":   email,
	})
	if err != nil {
		return err
	}

	for _, auth := range auths.Items {
		if err := req.Delete(&auth); err != nil {
			return err
		}
	}

	return nil
}

func (h *ProjectsHandler) AcceptPendingAuthorization(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		threadID  = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
		auths     v1.ThreadAuthorizationList
		thread    v1.Thread
	)

	email, ok := getEmail(req)
	if !ok {
		return types.NewErrBadRequest("email is required")
	}

	if err := req.Get(&thread, threadID); err != nil {
		return err
	}

	err := req.List(&auths, kclient.MatchingFields{
		"spec.threadID": threadID,
		"spec.userID":   email,
	})
	if err != nil {
		return err
	}

	if len(auths.Items) == 0 {
		return nil
	}

	auth := auths.Items[0]
	if !auth.Spec.Accepted {
		auth.Spec.Accepted = true
		return req.Update(&auth)
	}

	return nil
}

func (h *ProjectsHandler) ListPendingAuthorizations(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		result      types.ProjectAuthorizationList
	)

	email, ok := getEmail(req)
	if !ok {
		return req.Write(result)
	}

	agent, err := getAssistant(req, assistantID)
	if err != nil {
		return err
	}

	var threadAuths v1.ThreadAuthorizationList
	if err := req.List(&threadAuths, kclient.MatchingFields{
		"spec.userID":   email,
		"spec.accepted": "false",
	}); err != nil {
		return err
	}

	for _, threadAuth := range threadAuths.Items {
		var thread v1.Thread
		if err := req.Get(&thread, threadAuth.Spec.ThreadID); err != nil {
			return err
		}
		if thread.Spec.AgentName == agent.Name {
			project := convertProject(&thread, nil)
			result.Items = append(result.Items, types.ProjectAuthorization{
				Project: &project,
				Target:  threadAuth.Spec.UserID,
			})
		}
	}

	return req.Write(result)
}

func (h *ProjectsHandler) ListAuthorizations(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		result    types.ProjectAuthorizationList
	)

	var threadAuths v1.ThreadAuthorizationList
	if err := req.List(&threadAuths, kclient.MatchingFields{
		"spec.threadID": strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1),
	}); err != nil {
		return err
	}

	for _, threadAuth := range threadAuths.Items {
		result.Items = append(result.Items, types.ProjectAuthorization{
			Target:   threadAuth.Spec.UserID,
			Accepted: threadAuth.Spec.Accepted,
		})
	}

	return req.Write(result)
}

func (h *ProjectsHandler) UpdateProject(req api.Context) error {
	var (
		projectID = req.PathValue("project_id")
		project   types.ThreadManifest
	)

	if err := req.Read(&project); err != nil {
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)); err != nil {
		return err
	}

	project.Tools = thread.Spec.Manifest.Tools

	if !equality.Semantic.DeepEqual(thread.Spec.Manifest, project) {
		thread.Spec.Manifest = project
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

	var parentThread v1.Thread
	if thread.Spec.ParentThreadName != "" {
		if err := req.Get(&parentThread, thread.Spec.ParentThreadName); err == nil {
			return req.Write(convertProject(&thread, &parentThread))
		}
	}
	return req.Write(convertProject(&thread, nil))
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

	projects, err := h.getProjects(req, agent, req.UserIsAdmin() && req.URL.Query().Get("all") == "true")
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
			},
			AgentName: agent.Name,
			Project:   true,
			UserID:    req.User.GetUID(),
		},
	}

	if err := req.Create(thread); err != nil {
		return err
	}

	return req.WriteCreated(convertProject(thread, nil))
}

func getEmail(req api.Context) (string, bool) {
	if attr := req.User.GetExtra()["email"]; len(attr) > 0 {
		return attr[0], true
	}
	return "", false
}

func (h *ProjectsHandler) getProjects(req api.Context, agent *v1.Agent, all bool) (result types.ProjectList, err error) {
	var (
		threads v1.ThreadList
		auths   v1.ThreadAuthorizationList
		seen    = make(map[string]bool)
		fields  = kclient.MatchingFields{
			"spec.project": "true",
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

	if email, ok := getEmail(req); ok {
		err = req.List(&auths, kclient.MatchingFields{
			"spec.userID": email,
		})
		if err != nil {
			return result, err
		}

		for _, auth := range auths.Items {
			if seen[auth.Spec.ThreadID] || !auth.Spec.Accepted {
				continue
			}
			var thread v1.Thread
			if err := h.cachedClient.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: auth.Spec.ThreadID}, &thread); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return result, err
			}

			if !thread.Spec.Project || thread.Spec.AgentName != agent.Name {
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
	}

	return result, nil
}

func convertProject(thread *v1.Thread, parentThread *v1.Thread) types.Project {
	p := types.Project{
		Metadata: MetadataFrom(thread),
		ProjectManifest: types.ProjectManifest{
			ThreadManifest: thread.Spec.Manifest,
		},
		ParentID:        strings.Replace(thread.Spec.ParentThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		SourceProjectID: strings.Replace(thread.Spec.SourceThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		AssistantID:     thread.Spec.AgentName,
		Editor:          thread.IsEditor(),
		UserID:          thread.Spec.UserID,
		Capabilities:    types.ProjectCapabilities(thread.Spec.Capabilities),
	}

	// Include tools from parent project
	if parentThread != nil {
		p.Tools = append(p.Tools, parentThread.Spec.Manifest.Tools...)
	}

	p.Type = "project"
	p.ID = strings.Replace(p.ID, system.ThreadPrefix, system.ProjectPrefix, 1)
	return p
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
		"spec.userUID":          req.User.GetUID(),
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
		existingCredentials = make(map[string]struct{})
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

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credContextID},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		existingCredentials[cred.ToolName] = struct{}{}
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
		for _, credName := range toolRef.Status.Tool.CredentialNames {
			if _, ok := existingCredentials[credName]; !ok {
				exists = false
				break
			}
		}

		result.Items = append(result.Items, types.ProjectCredential{
			ToolID:   toolRef.Name,
			ToolName: toolRef.Status.Tool.Name,
			Icon:     toolRef.Status.Tool.Metadata["icon"],
			Exists:   exists,
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
	resp, err := runAuthForAgent(req.Context(), req.Storage, h.invoker, h.gptScript, &agent, credContext, tools, req.User.GetUID(), thread.Name)
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

	errs := removeToolCredentials(req.Context(), req.Storage, h.gptScript, credContext, agent.Namespace, tools)
	return errors.Join(errs...)
}
