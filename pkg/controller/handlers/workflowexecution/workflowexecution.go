package workflowexecution

import (
	"context"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/apiclient/types"
	log2 "github.com/gptscript-ai/otto/logger"
	"github.com/gptscript-ai/otto/pkg/controller/handlers/workflowstep"
	"github.com/gptscript-ai/otto/pkg/invoke"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	wclient "github.com/thedadams/workspace-provider/pkg/client"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = log2.Package()

type Handler struct {
	workspaceClient *wclient.Client
	invoker         *invoke.Invoker
}

func New(wc *wclient.Client, invoker *invoke.Invoker) *Handler {
	return &Handler{
		workspaceClient: wc,
		invoker:         invoker,
	}
}

func (h *Handler) Cleanup(req router.Request, resp router.Response) error {
	we := req.Object.(*v1.WorkflowExecution)
	if we.Status.ThreadName != "" {
		return kclient.IgnoreNotFound(req.Delete(&v1.Thread{
			ObjectMeta: metav1.ObjectMeta{
				Name:      we.Status.ThreadName,
				Namespace: we.Namespace,
			},
		}))
	}
	return nil
}

func (h *Handler) Run(req router.Request, resp router.Response) error {
	we := req.Object.(*v1.WorkflowExecution)

	switch we.Status.State {
	case v1.WorkflowStateError, v1.WorkflowStateComplete:
		resp.DisablePrune()
		return nil
	}

	var wf v1.Workflow
	if err := req.Get(&wf, we.Namespace, we.Spec.WorkflowName); err != nil {
		return err
	}

	// Wait for workspaces
	if wf.Status.KnowledgeWorkspaceName == "" ||
		wf.Status.WorkspaceName == "" {
		return nil
	}

	if we.Status.State != v1.WorkflowStateRunning {
		we.Status.State = v1.WorkflowStateRunning
		if err := req.Client.Status().Update(req.Ctx, we); err != nil {
			return err
		}
	}

	//if we.Status.WorkflowManifest == nil {
	if err := h.loadManifest(req, we); err != nil {
		return err
	}
	//}

	if we.Status.ThreadName == "" {
		if t, err := h.newThread(req.Ctx, req.Client, &wf, we); err != nil {
			return err
		} else {
			we.Status.ThreadName = t.Name
			if err := req.Client.Status().Update(req.Ctx, we); err != nil {
				return err
			}
		}
	}

	var (
		steps        []kclient.Object
		lastStepName = we.Spec.AfterWorkflowStepName
	)

	for _, step := range we.Status.WorkflowManifest.Steps {
		newStep := workflowstep.NewStep(we.Namespace, we.Name, lastStepName, step)
		steps = append(steps, newStep)
		lastStepName = newStep.Name
	}

	if we.Status.WorkflowManifest.Output != "" {
		newStep := workflowstep.NewStep(we.Namespace, we.Name, lastStepName, types.Step{
			ID:   "output",
			Step: we.Status.WorkflowManifest.Output,
		})
		steps = append(steps, newStep)
	}

	output, newState, err := h.getState(req.Ctx, req.Client, steps)
	if err != nil {
		return err
	}

	we.Status.State = newState
	if we.Status.WorkflowManifest.Output != "" {
		we.Status.Output = output
	}
	resp.Objects(steps...)
	return nil
}

func (h *Handler) getState(ctx context.Context, client kclient.Client, steps []kclient.Object) (string, v1.WorkflowState, error) {
	for i, obj := range steps {
		step := obj.(*v1.WorkflowStep).DeepCopy()
		if err := client.Get(ctx, router.Key(step.Namespace, step.Name), step); apierror.IsNotFound(err) {
			return "", v1.WorkflowStateRunning, nil
		} else if err != nil {
			return "", "", err
		}
		if step.Status.State == v1.WorkflowStepStateError {
			return "", v1.WorkflowStateError, nil
		}
		if len(steps)-1 == i && step.Status.State == v1.WorkflowStepStateComplete {
			var run v1.Run
			if err := client.Get(ctx, router.Key(step.Namespace, step.Status.LastRunName), &run); err != nil {
				return "", v1.WorkflowStateError, err
			}
			return run.Status.Output, v1.WorkflowStateComplete, nil
		}
	}

	return "", v1.WorkflowStateRunning, nil
}

func (h *Handler) loadManifest(req router.Request, we *v1.WorkflowExecution) error {
	var wf v1.Workflow
	if err := req.Get(&wf, we.Namespace, we.Spec.WorkflowName); err != nil {
		return err
	}

	we.Status.WorkflowManifest = &wf.Spec.Manifest
	return nil
	//return req.Client.Status().Update(req.Ctx, we)
}

func (h *Handler) newThread(ctx context.Context, c kclient.Client, wf *v1.Workflow, we *v1.WorkflowExecution) (*v1.Thread, error) {
	workspaceName := we.Spec.WorkspaceName
	if workspaceName == "" {
		workspaceName = wf.Status.WorkspaceName
	}

	var ws v1.Workspace
	if err := c.Get(ctx, router.Key(wf.Namespace, workspaceName), &ws); err != nil {
		return nil, err
	}

	var knowledgWs v1.Workspace
	if err := c.Get(ctx, router.Key(wf.Namespace, wf.Status.KnowledgeWorkspaceName), &knowledgWs); err != nil {
		return nil, err
	}

	return h.invoker.NewThread(ctx, c, wf.Namespace, invoke.NewThreadOptions{
		WorkflowName:          we.Spec.WorkflowName,
		WorkflowExecutionName: we.Name,
		WebhookName:           we.Spec.WebhookName,
		WebhookExecutionName:  we.Spec.WebhookExecutionName,
		WorkspaceIDs:          []string{ws.Status.WorkspaceID},
		KnowledgeWorkspaceIDs: []string{knowledgWs.Status.WorkspaceID},
	})
}
