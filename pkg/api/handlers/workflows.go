package handlers

import (
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/mvl"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/alias"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflow"
	"github.com/otto8-ai/otto8/pkg/invoke"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var log = mvl.Package()

type WorkflowHandler struct {
	gptscript *gptscript.GPTScript
	serverURL string
	invoker   *invoke.Invoker
}

func NewWorkflowHandler(gClient *gptscript.GPTScript, serverURL string, invoker *invoke.Invoker) *WorkflowHandler {
	return &WorkflowHandler{
		gptscript: gClient,
		serverURL: serverURL,
		invoker:   invoker,
	}
}

func (a *WorkflowHandler) Authenticate(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
	)

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	agent, err := render.Workflow(req.Context(), req.Storage, &workflow, render.WorkflowOptions{})
	if err != nil {
		return err
	}

	agent.Spec.Manifest.Prompt = "#!sys.echo\nDONE"
	if len(agent.Spec.Credentials) == 0 {
		return nil
	}

	resp, err := a.invoker.Agent(req.Context(), req.Storage, agent, "", invoke.Options{
		Synchronous:           true,
		ThreadCredentialScope: new(bool),
	})
	if err != nil {
		return err
	}
	defer resp.Close()

	req.ResponseWriter.Header().Set("X-Otto-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
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

	resp, err := convertWorkflow(wf, req)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
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
	wf := &v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(wf); err != nil {
		return err
	}

	resp, err := convertWorkflow(*wf, req)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func convertWorkflow(workflow v1.Workflow, req api.Context) (*types.Workflow, error) {
	var links []string
	if req.APIBaseURL != "" {
		alias := workflow.Name
		if workflow.Status.AliasAssigned && workflow.Spec.Manifest.Alias != "" {
			alias = workflow.Spec.Manifest.Alias
		}
		links = []string{"invoke", req.APIBaseURL + "/invoke/" + alias}
	}

	var embeddingModel string
	if len(workflow.Status.KnowledgeSetNames) > 0 {
		var ks v1.KnowledgeSet
		if err := req.Get(&ks, workflow.Status.KnowledgeSetNames[0]); err == nil {
			embeddingModel = ks.Status.TextEmbeddingModel
		} else {
			log.Warnf("failed to get KnowledgeSet %q for workflow %q: %v", workflow.Status.KnowledgeSetNames[0], workflow.Name, err)
		}
	}

	var aliasAssigned *bool
	if workflow.Generation == workflow.Status.AliasObservedGeneration {
		aliasAssigned = &workflow.Status.AliasAssigned
	}

	return &types.Workflow{
		Metadata:           MetadataFrom(&workflow, links...),
		WorkflowManifest:   workflow.Spec.Manifest,
		AliasAssigned:      aliasAssigned,
		AuthStatus:         workflow.Status.AuthStatus,
		TextEmbeddingModel: embeddingModel,
	}, nil
}

func (a *WorkflowHandler) ByID(req api.Context) error {
	var (
		workflow v1.Workflow
		id       = req.PathValue("id")
	)

	if err := alias.Get(req.Context(), req.Storage, &workflow, req.Namespace(), id); err != nil {
		return err
	}

	resp, err := convertWorkflow(workflow, req)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func (a *WorkflowHandler) List(req api.Context) error {
	var workflowList v1.WorkflowList
	if err := req.List(&workflowList); err != nil {
		return err
	}

	var resp types.WorkflowList
	for _, workflow := range workflowList.Items {
		convertedWorkflow, err := convertWorkflow(workflow, req)
		if err != nil {
			return err
		}

		resp.Items = append(resp.Items, *convertedWorkflow)
	}

	return req.Write(resp)
}

func (a *WorkflowHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}

	ref := req.PathValue("ref")
	authStatus := wf.Status.AuthStatus[ref]

	// If auth is not required, then don't continue.
	if authStatus.Required != nil && !*authStatus.Required {
		resp, err := convertWorkflow(wf, req)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	// if auth is already authenticated, then don't continue.
	if authStatus.Authenticated {
		resp, err := convertWorkflow(wf, req)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	credentialTool, err := v1.CredentialTool(req.Context(), req.Storage, req.Namespace(), ref)
	if err != nil {
		return err
	}

	if credentialTool == "" {
		// The only way to get here is if the controller hasn't set the field yet.
		if wf.Status.AuthStatus == nil {
			wf.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}

		authStatus.Required = &[]bool{false}[0]
		wf.Status.AuthStatus[ref] = authStatus
		resp, err := convertWorkflow(wf, req)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	oauthLogin := &v1.OAuthAppLogin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthAppLoginPrefix + wf.Name + ref,
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppLoginSpec{
			CredentialContext: wf.Name,
			ToolReference:     ref,
			OAuthApps:         wf.Spec.Manifest.OAuthApps,
		},
	}

	if err = req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err = wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) (bool, error) {
		return obj.Status.External.Authenticated || obj.Status.External.Error != "" || obj.Status.External.URL != "", nil
	}, wait.Option{
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure credential for workflow %q: %w", wf.Name, err)
	}

	// Don't need to actually update the knowledge ref, there is a controller that will do that.
	if wf.Status.AuthStatus == nil {
		wf.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
	}
	wf.Status.AuthStatus[ref] = oauthLogin.Status.External

	resp, err := convertWorkflow(wf, req)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
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

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, agent, a.serverURL, render.AgentOptions{})
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
