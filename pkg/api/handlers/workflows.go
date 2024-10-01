package handlers

import (
	"fmt"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflow"
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
		wf       v1.Workflow
		manifest types.WorkflowManifest
	)

	if err := req.Read(&manifest); err != nil {
		return err
	}

	manifest = workflow.PopulateIDs(manifest)

	if err := req.Get(&wf, id); err != nil {
		return err
	}

	wf.Spec.Manifest = manifest
	if err := req.Update(&wf); err != nil {
		return err
	}

	return req.Write(convertWorkflow(wf, api.GetURLPrefix(req)))
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
	var manifest types.WorkflowManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}
	manifest = workflow.PopulateIDs(manifest)
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
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
		refName := workflow.Name
		if workflow.Status.External.RefNameAssigned && workflow.Spec.Manifest.RefName != "" {
			refName = workflow.Spec.Manifest.RefName
		}
		links = []string{"invoke", prefix + "/invoke/" + refName}
	}
	return &types.Workflow{
		Metadata:               MetadataFrom(&workflow, links...),
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

	return listFiles(req.Context(), req, a.workspaceClient, workflow.Status.WorkspaceName)
}

func (a *WorkflowHandler) UploadFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.workspaceClient, workflow.Status.WorkspaceName); err != nil {
		return err
	}

	req.WriteHeader(http.StatusCreated)
	return nil
}

func (a *WorkflowHandler) DeleteFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)

	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	return deleteFile(req.Context(), req, a.workspaceClient, workflow.Status.WorkspaceName)
}

func (a *WorkflowHandler) Knowledge(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}
	return listKnowledgeFiles(req, wf.Status.KnowledgeWorkspaceName)
}

func (a *WorkflowHandler) UploadKnowledge(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}
	return uploadKnowledge(req, a.workspaceClient, wf.Status.KnowledgeWorkspaceName)
}

func (a *WorkflowHandler) DeleteKnowledge(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}
	return deleteKnowledge(req, req.PathValue("file"), wf.Status.KnowledgeWorkspaceName)
}

func (a *WorkflowHandler) IngestKnowledge(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}
	return ingestKnowledge(req, a.workspaceClient, wf.Status.KnowledgeWorkspaceName)
}

func (a *WorkflowHandler) CreateRemoteKnowledgeSource(req api.Context) error {
	return createRemoteKnowledgeSource(req, req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) UpdateRemoteKnowledgeSource(req api.Context) error {
	return updateRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) ReSyncRemoteKnowledgeSource(req api.Context) error {
	return reSyncRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) GetRemoteKnowledgeSources(req api.Context) error {
	return getRemoteKnowledgeSourceForParent(req, req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) DeleteRemoteKnowledgeSource(req api.Context) error {
	return deleteRemoteKnowledgeSource(req, req.PathValue("id"), req.PathValue("workflow_id"), new(v1.Workflow))
}

func (a *WorkflowHandler) Script(req api.Context) error {
	var (
		id     = req.Request.PathValue("id")
		stepID = req.Request.URL.Query().Get("step")
		wf     v1.Workflow
	)
	if err := req.Get(&wf, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	step := types.FindStep(&wf.Spec.Manifest, stepID)
	agent, err := render.Workflow(req.Context(), req.Storage, &wf, render.WorkflowOptions{
		Step: step,
	})
	if err != nil {
		return err
	}

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
