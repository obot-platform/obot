package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/workflow"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const fieldSelector = "spec.workflow"

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
		tools    []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for authentication")
	}

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	agent, err := render.Workflow(req.Context(), req.Storage, &workflow, render.WorkflowOptions{})
	if err != nil {
		return err
	}

	resp, err := runAuthForAgent(req.Context(), req.Storage, a.invoker, a.gptscript, agent, id, tools)
	if err != nil {
		return err
	}
	defer func() {
		resp.Close()
		if kickErr := kickWorkflow(req.Context(), req.Storage, &workflow); kickErr != nil && err == nil {
			err = fmt.Errorf("failed to update workflow status: %w", kickErr)
		}
	}()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (a *WorkflowHandler) DeAuthenticate(req api.Context) error {
	var (
		id    = req.PathValue("id")
		wf    v1.Workflow
		tools []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for de-authentication")
	}

	if err := req.Get(&wf, id); err != nil {
		return err
	}

	errs := removeToolCredentials(req.Context(), req.Storage, a.gptscript, id, wf.Namespace, tools)

	if err := kickWorkflow(req.Context(), req.Storage, &wf); err != nil {
		errs = append(errs, fmt.Errorf("failed to update workflow status: %w", err))
	}

	return errors.Join(errs...)
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

	if wf.Spec.Manifest.Model != manifest.Model && manifest.Model != "" {
		// Get the model to ensure it is active
		var model v1.Model
		if err := req.Get(&model, manifest.Model); err != nil {
			return err
		}

		if !model.Spec.Manifest.Active {
			return types.NewErrBadRequest("workflow cannot use inactive model %q", manifest.Model)
		}
	}

	wf.Spec.Manifest = manifest
	if err := req.Update(&wf); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(wf.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, wf.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get workflow knowledge set: %w", err)
		}
	}

	resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func (a *WorkflowHandler) Delete(req api.Context) error {
	var (
		id             = req.PathValue("id")
		deleteTriggers = req.Request.URL.Query().Get("delete-triggers")
	)

	if deleteTriggers == "true" {
		listOptions := &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				fieldSelector: id,
			}),
			Namespace: req.Namespace(),
		}

		var webhooks v1.WebhookList
		if err := req.List(&webhooks, listOptions); err != nil {
			return err
		}

		for _, webhook := range webhooks.Items {
			if err := req.Delete(&webhook); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}

		var cronjobs v1.CronJobList
		if err := req.List(&cronjobs, listOptions); err != nil {
			return err
		}

		for _, cronjob := range cronjobs.Items {
			if err := req.Delete(&cronjob); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}

		var emailReceivers v1.EmailReceiverList
		if err := req.List(&emailReceivers, listOptions); err != nil {
			return err
		}

		for _, emailReceiver := range emailReceivers.Items {
			if err := req.Delete(&emailReceiver); err != nil && !apierror.IsNotFound(err) {
				return err
			}
		}
	}

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

	if manifest.Model != "" {
		// Get the model to ensure it is active
		var model v1.Model
		if err := req.Get(&model, manifest.Model); err != nil {
			return err
		}

		if !model.Spec.Manifest.Active {
			return types.NewErrBadRequest("workflow cannot use inactive model %q", manifest.Model)
		}
	}

	manifest = workflow.PopulateIDs(manifest)
	wf := &v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.WorkflowFinalizer},
		},
		Spec: v1.WorkflowSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(wf); err != nil {
		return err
	}

	// The workflow won't have a knowledge set associated to it on create, so send the text embedding model as an empty string.
	resp, err := convertWorkflow(*wf, "", req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func convertWorkflow(workflow v1.Workflow, textEmbeddingModel, baseURL string) (*types.Workflow, error) {
	var links []string
	if baseURL != "" {
		alias := workflow.Name
		if workflow.Status.AliasAssigned && workflow.Spec.Manifest.Alias != "" {
			alias = workflow.Spec.Manifest.Alias
		}
		links = []string{"invoke", baseURL + "/invoke/" + alias}
	}

	var (
		aliasAssigned *bool
		toolInfos     *map[string]types.ToolInfo
	)
	if workflow.Generation == workflow.Status.ObservedGeneration {
		aliasAssigned = &workflow.Status.AliasAssigned
		toolInfos = &workflow.Status.ToolInfo
	}

	return &types.Workflow{
		Metadata:           MetadataFrom(&workflow, links...),
		WorkflowManifest:   workflow.Spec.Manifest,
		ThreadID:           workflow.Spec.ThreadName,
		AliasAssigned:      aliasAssigned,
		AuthStatus:         workflow.Status.AuthStatus,
		ToolInfo:           toolInfos,
		TextEmbeddingModel: textEmbeddingModel,
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

	var knowledgeSet v1.KnowledgeSet
	if len(workflow.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, workflow.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get workflow knowledge set: %w", err)
		}
	}

	resp, err := convertWorkflow(workflow, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
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

	var knowledgeSets v1.KnowledgeSetList
	if err := req.List(&knowledgeSets); err != nil {
		return fmt.Errorf("failed to get agent knowledge sets: %w", err)
	}

	textEmbeddingModels := make(map[string]string, len(knowledgeSets.Items))
	for _, knowledgeSet := range knowledgeSets.Items {
		textEmbeddingModels[knowledgeSet.Name] = knowledgeSet.Status.TextEmbeddingModel
	}

	var textEmbeddingModel string
	resp := make([]types.Workflow, 0, len(workflowList.Items))
	for _, workflow := range workflowList.Items {
		if len(workflow.Status.KnowledgeSetNames) > 0 {
			textEmbeddingModel = textEmbeddingModels[workflow.Status.KnowledgeSetNames[0]]
		} else {
			textEmbeddingModel = ""
		}
		convertedWorkflow, err := convertWorkflow(workflow, textEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}

		resp = append(resp, *convertedWorkflow)
	}

	return req.Write(types.WorkflowList{Items: resp})
}

func (a *WorkflowHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(wf.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, wf.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get workflow knowledge set: %w", err)
		}
	}

	ref := req.PathValue("ref")
	authStatus := wf.Status.AuthStatus[ref]

	// If auth is not required, then don't continue.
	if authStatus.Required != nil && !*authStatus.Required {
		resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	var toolReference v1.ToolReference
	if err := req.Get(&toolReference, ref); err != nil {
		return fmt.Errorf("failed to get tool reference %v", ref)
	}
	if toolReference.Status.Tool == nil {
		return types.NewErrHttp(http.StatusTooEarly, "tool reference is not ready yet")
	}

	if len(toolReference.Status.Tool.Credentials) == 0 {
		// The only way to get here is if the controller hasn't set the field yet.
		if wf.Status.AuthStatus == nil {
			wf.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}

		authStatus.Required = &[]bool{false}[0]
		wf.Status.AuthStatus[ref] = authStatus
		resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
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
			OAuthApps:         []string{toolReference.Status.Tool.Metadata["oauth"]},
		},
	}

	if err := req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err := wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) (bool, error) {
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

	resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func (a *WorkflowHandler) WorkflowExecutions(req api.Context) error {
	var (
		id   = req.PathValue("id")
		wfes v1.WorkflowExecutionList
	)
	if err := req.List(&wfes, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.workflowName": id,
		}),
	}); err != nil {
		log.Warnf("failed to list workflow executions: %v", err)
		return err
	}

	var resp types.WorkflowExecutionList
	for _, we := range wfes.Items {
		resp.Items = append(resp.Items, convertWorkflowExecution(we))
	}

	return req.Write(resp)
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
			Text: "!obot-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}

func kickWorkflow(ctx context.Context, c kclient.Client, wf *v1.Workflow) error {
	if wf.Annotations[v1.WorkflowSyncAnnotation] != "" {
		delete(wf.Annotations, v1.WorkflowSyncAnnotation)
	} else {
		if wf.Annotations == nil {
			wf.Annotations = make(map[string]string)
		}
		wf.Annotations[v1.WorkflowSyncAnnotation] = "true"
	}

	return c.Update(ctx, wf)
}
