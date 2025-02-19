package handlers

import (
	"errors"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/projects"
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
			project := convertProject(&thread)
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

	return req.Write(convertProject(&thread))
}

func (h *ProjectsHandler) GetProject(req api.Context) error {
	var (
		thread    v1.Thread
		projectID = strings.Replace(req.PathValue("project_id"), system.ProjectPrefix, system.ThreadPrefix, 1)
	)
	if err := req.Get(&thread, projectID); err != nil {
		return err
	}
	return req.Write(convertProject(&thread))
}

func (h *ProjectsHandler) ListProjects(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		agent       *v1.Agent
		err         error
	)
	if assistantID != "" {
		agent, err = getAssistant(req, assistantID)
		if err != nil {
			return err
		}
	}

	projects, err := h.getProjects(req, agent)
	if err != nil {
		return err
	}

	return req.Write(projects)
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

func getThreadTemplate(req api.Context, id string) (*v1.ThreadTemplate, error) {
	if system.IsThreadTemplateID(id) {
		var template v1.ThreadTemplate
		if err := req.Get(&template, id); err != nil {
			return nil, err
		}
		return &template, nil
	}

	var list v1.ThreadTemplateList
	if err := req.List(&list, kclient.MatchingFields{
		"status.publicID": id,
	}); err != nil {
		return nil, err
	}

	if len(list.Items) == 0 {
		return nil, types.NewErrNotFound("template %s not found", id)
	}

	return &list.Items[0], nil
}

func (h *ProjectsHandler) CreateProject(req api.Context) error {
	var (
		assistantID    = req.PathValue("assistant_id")
		templateID     = req.PathValue("template_id")
		agent          *v1.Agent
		threadTemplate *v1.ThreadTemplate
		err            error
	)
	if assistantID != "" {
		agent, err = getAssistant(req, assistantID)
		if err != nil {
			return err
		}
	}

	if templateID != "" {
		threadTemplate, err = getThreadTemplate(req, templateID)
		if err != nil {
			return err
		}
		agent = &v1.Agent{}
		if err := req.Get(agent, threadTemplate.Status.AgentName); err != nil {
			return err
		}
	}

	var project types.ProjectManifest
	if err := req.Read(&project); err != nil {
		return err
	}

	thread, err := invoke.CreateProjectForAgent(req.Context(), req.Storage, agent, threadTemplate, project.Name, req.User.GetUID())
	if err != nil {
		return err
	}

	return req.WriteCreated(convertProject(thread))
}

func getEmail(req api.Context) (string, bool) {
	if attr := req.User.GetExtra()["email"]; len(attr) > 0 {
		return attr[0], true
	}
	return "", false
}

func (h *ProjectsHandler) getProjects(req api.Context, agent *v1.Agent) (result types.ProjectList, err error) {
	var (
		threads v1.ThreadList
		auths   v1.ThreadAuthorizationList
		seen    = make(map[string]bool)
		fields  = kclient.MatchingFields{
			"spec.project": "true",
			"spec.userUID": req.User.GetUID(),
		}
	)

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
		result.Items = append(result.Items, convertProject(&thread))
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

			result.Items = append(result.Items, convertProject(&thread))
			seen[auth.Spec.ThreadID] = true
		}
	}

	return result, nil
}

func convertProject(thread *v1.Thread) types.Project {
	p := types.Project{
		Metadata: MetadataFrom(thread),
		ProjectManifest: types.ProjectManifest{
			ThreadManifest: thread.Spec.Manifest,
		},
		AssistantID: thread.Spec.AgentName,
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

	thread, err := invoke.CreateThreadForProject(req.Context(), req.Storage, projectThread, req.User.GetUID())
	if err != nil {
		return err
	}

	return req.WriteCreated(convertThread(*thread))
}

func (h *ProjectsHandler) ListProjectThreads(req api.Context) error {
	var (
		threads v1.ThreadList
	)

	projectThread, err := h.getProjectThread(req)
	if err != nil {
		return err
	}

	if err := req.List(&threads,
		kclient.MatchingFields{
			"spec.project":          "false",
			"spec.parentThreadName": projectThread.Name,
			"spec.userUID":          req.User.GetUID(),
		}); err != nil {
		return err
	}

	var result types.ThreadList
	for _, thread := range threads.Items {
		result.Items = append(result.Items, convertThread(thread))
	}

	return req.Write(result)
}

func (h *ProjectsHandler) ListCredentials(req api.Context) error {
	var (
		tools               = make(map[string]struct{})
		existingCredentials = make(map[string]struct{})
		result              types.ProjectCredentialList
	)

	agent, err := getAssistant(req, req.PathValue("assistant_id"))
	if err != nil {
		return err
	}

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	allTools := slices.Concat(agent.Spec.Manifest.Tools,
		agent.Spec.Manifest.DefaultThreadTools,
		agent.Spec.Manifest.AvailableThreadTools,
		thread.Spec.Manifest.Tools)
	for _, tool := range allTools {
		tools[tool] = struct{}{}
	}

	credContextIDs, err := projects.ThreadIDs(req.Context(), req.Storage, thread)
	if err != nil {
		return err
	}

	credContextIDs = append(credContextIDs, thread.Spec.AgentName)
	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credContextIDs,
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

func (h *ProjectsHandler) Authenticate(req api.Context) (err error) {
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

	resp, err := runAuthForAgent(req.Context(), req.Storage, h.invoker, h.gptScript, &agent, thread.Name, tools)
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (h *ProjectsHandler) DeAuthenticate(req api.Context) error {
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

	errs := removeToolCredentials(req.Context(), req.Storage, h.gptScript, thread.Name, agent.Namespace, tools)
	return errors.Join(errs...)
}

func (h *ProjectsHandler) ListTemplates(req api.Context) error {
	var (
		templates v1.ThreadTemplateList
		result    types.ProjectTemplateList
	)

	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	if err := req.List(&templates, kclient.MatchingFields{
		"spec.projectThreadName": thread.Name,
	}); err != nil {
		return err
	}

	for _, template := range templates.Items {
		result.Items = append(result.Items, convertThreadTemplate(template))
	}

	return req.Write(result)
}

func (h *ProjectsHandler) DeleteTemplate(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var template v1.ThreadTemplate
	if err := req.Get(&template, req.PathValue("id")); err != nil {
		return err
	}

	if template.Spec.ProjectThreadName != thread.Name {
		return types.NewErrNotFound("template %s not found", req.PathValue("id"))
	}

	return req.Delete(&template)
}

func (h *ProjectsHandler) GetTemplate(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	var template v1.ThreadTemplate
	if err := req.Get(&template, req.PathValue("id")); err != nil {
		return err
	}

	if template.Spec.ProjectThreadName != thread.Name {
		return types.NewErrNotFound("template %s not found", req.PathValue("id"))
	}

	return req.Write(convertThreadTemplate(template))
}

func (h *ProjectsHandler) CreateTemplate(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	template := v1.ThreadTemplate{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadTemplatePrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ThreadTemplateSpec{
			ProjectThreadName: thread.Name,
			UserID:            req.User.GetUID(),
		},
	}
	if err := req.Create(&template); err != nil {
		return err
	}

	return req.WriteCreated(convertThreadTemplate(template))
}

func convertThreadTemplate(template v1.ThreadTemplate) types.ProjectTemplate {
	return types.ProjectTemplate{
		Metadata:       MetadataFrom(&template),
		ThreadManifest: template.Status.Manifest,
		Tasks:          template.Status.Tasks,
		AssistantID:    template.Status.AgentName,
		PublicID:       template.Status.PublicID,
		Ready:          template.Status.Ready,
	}
}
