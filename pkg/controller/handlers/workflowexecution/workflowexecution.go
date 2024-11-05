package workflowexecution

import (
	"context"
	"time"

	"github.com/otto8-ai/nah/pkg/apply"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/controller/handlers/workflowstep"
	"github.com/otto8-ai/otto8/pkg/invoke"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	invoker *invoke.Invoker
}

func New(invoker *invoke.Invoker) *Handler {
	return &Handler{
		invoker: invoker,
	}
}

func (h *Handler) Run(req router.Request, _ router.Response) error {
	var (
		we = req.Object.(*v1.WorkflowExecution)
	)

	if we.Status.State.IsTerminal() {
		if we.Spec.WorkflowGeneration != we.Status.WorkflowGeneration {
			we.Status.State = types.WorkflowStatePending
			we.Status.EndTime = nil
		}
		return nil
	}

	var wf v1.Workflow
	if err := req.Get(&wf, we.Namespace, we.Spec.WorkflowName); err != nil {
		return err
	}

	// Wait for workspaces
	if wf.Status.WorkspaceName == "" {
		return nil
	}

	if err := h.loadManifest(req, we); err != nil {
		return err
	}

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
		newStep := workflowstep.NewStep(we.Namespace, we.Name, lastStepName, we.Spec.WorkflowGeneration, step)
		steps = append(steps, newStep)
		lastStepName = newStep.Name
	}

	if we.Status.WorkflowManifest.Output != "" {
		newStep := workflowstep.NewStep(we.Namespace, we.Name, lastStepName, we.Spec.WorkflowGeneration, types.Step{
			ID:   "output",
			Step: we.Status.WorkflowManifest.Output,
		})
		steps = append(steps, newStep)
	}

	_, output, newState, err := workflowstep.GetStateFromSteps(req.Ctx, req.Client, we.Spec.WorkflowGeneration, steps...)
	if err != nil {
		return err
	}

	if newState.IsBlocked() {
		we.Status.State = newState
		we.Status.Error = output
		return nil
	}

	if newState == types.WorkflowStateComplete {
		we.Status.Output = output
	} else if newState == types.WorkflowStateError {
		we.Status.Error = output
	}

	we.Status.State = newState
	we.Status.WorkflowGeneration = we.Spec.WorkflowGeneration
	if we.Status.State.IsTerminal() && we.Status.EndTime == nil {
		we.Status.EndTime = &metav1.Time{Time: time.Now()}
	}

	return apply.New(req.Client).Apply(req.Ctx, req.Object, steps...)
}

func (h *Handler) loadManifest(req router.Request, we *v1.WorkflowExecution) error {
	var wf v1.Workflow
	if err := req.Get(&wf, we.Namespace, we.Spec.WorkflowName); err != nil {
		return err
	}

	we.Status.WorkflowManifest = &wf.Spec.Manifest
	return nil
}

func (h *Handler) newThread(ctx context.Context, c kclient.Client, wf *v1.Workflow, we *v1.WorkflowExecution) (*v1.Thread, error) {
	workspaceName := we.Spec.WorkspaceName
	if workspaceName == "" {
		workspaceName = wf.Status.WorkspaceName
	}

	thread := v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    wf.Namespace,
			GenerateName: system.ThreadPrefix,
		},
		Spec: v1.ThreadSpec{
			ParentThreadName:      we.Spec.ParentThreadName,
			WorkflowName:          we.Spec.WorkflowName,
			WorkflowExecutionName: we.Name,
			WebhookName:           we.Spec.WebhookName,
			CronJobName:           we.Spec.CronJobName,
			FromWorkspaceNames:    []string{workspaceName},
		},
	}

	return &thread, c.Create(ctx, &thread)
}
