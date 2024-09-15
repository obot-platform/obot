package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
	"github.com/gptscript-ai/otto/pkg/render"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	"github.com/thedadams/workspace-provider/pkg/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkflowHandler struct {
	workspaceClient   *client.Client
	workspaceProvider string
}

func NewWorkflowHandler(wc *client.Client, wp string) *WorkflowHandler {
	return &WorkflowHandler{
		workspaceClient:   wc,
		workspaceProvider: wp,
	}
}

func (a *WorkflowHandler) Update(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
		manifest v1.WorkflowManifest
	)

	if err := req.Read(&manifest); err != nil {
		return err
	}

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	workflow.Spec.Manifest = manifest
	if err := req.Update(&workflow); err != nil {
		return err
	}

	return req.Write(convertWorkflow(workflow, api.GetURLPrefix(req)))
}

func (a *WorkflowHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *WorkflowHandler) Create(req api.Context) error {
	var manifest v1.WorkflowManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.WorkflowFinalizer},
		},
		Spec: v1.WorkflowSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&workflow); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return req.Write(convertWorkflow(workflow, api.GetURLPrefix(req)))
}

func convertWorkflow(workflow v1.Workflow, prefix string) *types.Workflow {
	var links []string
	if prefix != "" {
		slug := workflow.Name
		if workflow.Status.External.SlugAssigned && workflow.Spec.Manifest.Slug != "" {
			slug = workflow.Spec.Manifest.Slug
		}
		links = []string{"invoke", prefix + "/invoke/" + slug}
	}
	return &types.Workflow{
		Metadata:               types.MetadataFrom(&workflow, links...),
		WorkflowManifest:       workflow.Spec.Manifest,
		WorkflowExternalStatus: workflow.Status.External,
	}
}

func (a *WorkflowHandler) ByID(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return req.Write(convertWorkflow(workflow, api.GetURLPrefix(req)))
}

func (a *WorkflowHandler) List(req api.Context) error {
	var workflowList v1.WorkflowList
	if err := req.List(&workflowList); err != nil {
		return err
	}

	var resp types.WorkflowList
	for _, workflow := range workflowList.Items {
		resp.Items = append(resp.Items, *convertWorkflow(workflow, api.GetURLPrefix(req)))
	}

	return req.Write(resp)
}

func (a *WorkflowHandler) Files(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, workflow.Status.Workspace.WorkspaceID)
}

func (a *WorkflowHandler) UploadFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	return uploadFile(req.Context(), req, a.workspaceClient, workflow.Status.Workspace.WorkspaceID)
}

func (a *WorkflowHandler) DeleteFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		filename = req.PathValue("file")
		workflow v1.Workflow
	)

	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.workspaceClient, workflow.Status.Workspace.WorkspaceID, filename)
}

func (a *WorkflowHandler) Knowledge(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	return listFiles(req.Context(), req, a.workspaceClient, workflow.Status.KnowledgeWorkspace.KnowledgeWorkspaceID)
}

func (a *WorkflowHandler) UploadKnowledge(req api.Context) error {
	return uploadKnowledge(req, a.workspaceClient, req.PathValue("id"), new(v1.Workflow))
}

func (a *WorkflowHandler) DeleteKnowledge(req api.Context) error {
	return deleteKnowledge(req, a.workspaceClient, req.PathValue("file"), req.PathValue("id"), new(v1.Workflow))
}

func (a *WorkflowHandler) IngestKnowledge(req api.Context) error {
	return ingestKnowledge(req, a.workspaceClient, req.PathValue("id"), new(v1.Workflow))
}

func (a *WorkflowHandler) CreateOnedriveLinks(req api.Context) error {
	return createOneDriveLinks(req, req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) UpdateOnedriveLinks(req api.Context) error {
	return updateOneDriveLinks(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) ReSyncOnedriveLinks(req api.Context) error {
	return reSyncOneDriveLinks(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) GetOnedriveLinks(req api.Context) error {
	return getOneDriveLinksForParent(req, req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) DeleteOnedriveLinks(req api.Context) error {
	return deleteOneDriveLinks(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) Script(req api.Context) error {
	var (
		id       = req.Request.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	agent := render.Workflow(&workflow, render.WorkflowOptions{})

	tools, _, err := render.Agent(req.Context(), req.Storage, agent, render.AgentOptions{})
	if err != nil {
		return err
	}

	script, err := req.GPTClient.Fmt(req.Context(), gptscript.ToolDefsToNodes(tools))
	if err != nil {
		return err
	}

	return req.Write(script)
}
