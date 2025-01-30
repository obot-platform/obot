package handlers

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectsHandler struct {
	cachedClient kclient.Client
}

func NewProjectsHandler(cachedClient kclient.Client) *ProjectsHandler {
	return &ProjectsHandler{cachedClient: cachedClient}
}

func (h *ProjectsHandler) ListProjects(req api.Context) error {
	var assistantID = req.PathValue("assistant_id")
	agent, err := getAssistant(req, assistantID)
	if err != nil {
		return err
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
	var assistantID = req.PathValue("assistant_id")
	agent, err := getAssistant(req, assistantID)
	if err != nil {
		return err
	}

	var project types.ProjectManifest
	if err := req.Read(&project); err != nil {
		return err
	}

	thread, err := invoke.CreateProjectForAgent(req.Context(), req.Storage, agent, project.Name, req.User.GetUID())
	if err != nil {
		return err
	}

	return req.WriteCreated(convertProject(thread))
}

func (h *ProjectsHandler) getProjects(req api.Context, agent *v1.Agent) (result types.ProjectList, err error) {
	var (
		threads v1.ThreadList
		auths   v1.ThreadAuthorizationList
		seen    = make(map[string]bool)
	)

	err = req.Storage.List(req.Context(), &threads, kclient.InNamespace(agent.Namespace), kclient.MatchingFields{
		"spec.project":   "true",
		"spec.agentName": agent.Name,
		"spec.userUID":   req.User.GetUID(),
	})
	if err != nil {
		return result, err
	}

	for _, thread := range threads.Items {
		seen[thread.Name] = true
		result.Items = append(result.Items, convertProject(&thread))
	}

	err = req.Storage.List(req.Context(), &auths, kclient.InNamespace(agent.Namespace), kclient.MatchingFields{
		"spec.userID": req.User.GetUID(),
	})
	if err != nil {
		return result, err
	}

	for _, auth := range auths.Items {
		if seen[auth.Spec.ThreadID] {
			continue
		}
		var thread v1.Thread
		if err := h.cachedClient.Get(req.Context(), kclient.ObjectKey{Namespace: agent.Namespace, Name: auth.Spec.ThreadID}, &thread); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return result, err
		}

		if !thread.Spec.Project || thread.Spec.AgentName != agent.Name {
			continue
		}

		result.Items = append(result.Items, convertProject(&thread))
		seen[auth.Spec.ThreadID] = true
	}

	return result, nil
}

func convertProject(thread *v1.Thread) types.Project {
	p := types.Project{
		Metadata: MetadataFrom(thread),
		ProjectManifest: types.ProjectManifest{
			Name: thread.Spec.Manifest.Description,
		},
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
		projectID = req.PathValue("project_id")
		threads   v1.ThreadList
	)

	if err := req.Storage.List(req.Context(), &threads, kclient.InNamespace(req.Namespace()),
		kclient.MatchingFields{
			"spec.project":          "false",
			"spec.parentThreadName": strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1),
		}); err != nil {
		return err
	}

	var result types.ThreadList
	for _, thread := range threads.Items {
		result.Items = append(result.Items, convertThread(thread))
	}

	return req.Write(result)
}
