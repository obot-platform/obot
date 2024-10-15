package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/api/server"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflow"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WorkflowHandler struct {
	gptscript         *gptscript.GPTScript
	workspaceProvider string
}

func NewWorkflowHandler(gClient *gptscript.GPTScript, wp string) *WorkflowHandler {
	return &WorkflowHandler{
		gptscript:         gClient,
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

	return req.Write(convertWorkflow(wf, server.GetURLPrefix(req)))
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
	return req.Write(convertWorkflow(workflow, server.GetURLPrefix(req)))
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

	return req.Write(convertWorkflow(workflow, server.GetURLPrefix(req)))
}

func (a *WorkflowHandler) List(req api.Context) error {
	var workflowList v1.WorkflowList
	if err := req.List(&workflowList); err != nil {
		return err
	}

	var resp types.WorkflowList
	for _, workflow := range workflowList.Items {
		resp.Items = append(resp.Items, *convertWorkflow(workflow, server.GetURLPrefix(req)))
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

	return listFiles(req.Context(), req, a.gptscript, workflow.Status.WorkspaceName)
}

func (a *WorkflowHandler) UploadFile(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)
	if err := req.Get(&workflow, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	if err := uploadFile(req.Context(), req, a.gptscript, workflow.Status.WorkspaceName); err != nil {
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

	return deleteFile(req.Context(), req, a.gptscript, workflow.Status.WorkspaceName)
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

	step, _ := types.FindStep(&wf.Spec.Manifest, stepID)
	agent, err := render.Workflow(req.Context(), req.Storage, &wf, render.WorkflowOptions{
		Step: step,
	})
	if err != nil {
		return err
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, agent, render.AgentOptions{})
	if err != nil {
		return err
	}

	nodes := gptscript.ToolDefsToNodes(tools)
	nodes = append(nodes, gptscript.Node{
		TextNode: &gptscript.TextNode{
			Text: "!otto-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}
