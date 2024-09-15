package workflowstep

import (
	"slices"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/pkg/render"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *Handler) RunSubflow(req router.Request, resp router.Response) error {
	step := req.Object.(*v1.WorkflowStep)

	if step.Spec.SubFlow == nil {
		return nil
	}

	var wf v1.Workflow
	if err := req.Get(&wf, step.Namespace, step.Spec.WorkflowName); err != nil {
		return err
	}

	wfs, err := render.WorkflowByName(req.Ctx, req.Client, req.Namespace)
	if err != nil {
		return err
	}

	wf, ok := wfs[step.Spec.SubFlow.Workflow]
	if !ok {
		return nil
	}

	wfe := &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.WorkflowExecutionPrefix + step.Name,
			Namespace: step.Namespace,
		},
		Spec: v1.WorkflowExecutionSpec{
			Input:                 step.Spec.Step.Input,
			WorkflowName:          wf.Name,
			AfterWorkflowStepName: step.Spec.AfterWorkflowStepName,
			WorkspaceID:           wf.Status.Workspace.WorkspaceID,
		},
	}
	resp.Objects(wfe)

	out, err := h.getSubflowOutput(req, wfe)
	if err != nil {
		return err
	}

	if out != "" {
		stepPath := append(step.Spec.Path, "input-output")
		stepName := name.SafeHashConcatName(slices.Concat([]string{step.Spec.WorkflowExecutionName}, stepPath)...)
		invoke := &v1.WorkflowStep{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stepName,
				Namespace: step.Namespace,
			},
			Spec: v1.WorkflowStepSpec{
				ParentWorkflowStepName: step.Name,
				AfterWorkflowStepName:  step.Name,
				NoWaitForAfterComplete: true,
				WorkflowName:           step.Spec.WorkflowName,
				WorkflowExecutionName:  step.Spec.WorkflowExecutionName,
				ThreadName:             step.Spec.ThreadName,
				Step: v1.Step{
					Input: out,
				},
			},
		}
		resp.Objects(invoke)

		step.Status.LastRunName, err = h.getLastRun(req, invoke)
		if err != nil {
			return err
		}
		if step.Status.LastRunName != "" {
			step.Status.State = v1.WorkflowStepStateComplete
		}
	}

	return nil
}

func (h *Handler) getLastRun(req router.Request, step *v1.WorkflowStep) (string, error) {
	var check v1.WorkflowStep
	if err := req.Get(&check, step.Namespace, step.Name); apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	if check.Status.State != v1.WorkflowStepStateComplete {
		return "", nil
	}

	return check.Status.LastRunName, nil
}

func (h *Handler) getSubflowOutput(req router.Request, wfe *v1.WorkflowExecution) (string, error) {
	var (
		check v1.WorkflowExecution
	)

	if err := req.Get(&check, wfe.Namespace, wfe.Name); apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	if check.Status.External.State != v1.WorkflowStateComplete {
		return "", nil
	}

	return check.Status.External.Output, nil
}
