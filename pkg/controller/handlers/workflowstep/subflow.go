package workflowstep

import (
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *Handler) RunSubflow(req router.Request, _ router.Response) error {
	step := req.Object.(*v1.WorkflowStep)

	if step.Status.State != types.WorkflowStateSubCall {
		return nil
	}

	// The runs maybe zero on a rerun, reset state to pending
	if len(step.Status.RunNames) == 0 {
		step.Status.State = types.WorkflowStatePending
		return nil
	}

	wfs, err := render.WorkflowByName(req.Ctx, req.Client, req.Namespace)
	if err != nil {
		return err
	}

	for i, subCall := range step.Status.SubCalls {
		if len(step.Status.RunNames) > i+1 {
			continue
		}

		wf, ok := wfs[subCall.Workflow]
		if !ok {
			return nil
		}

		wfe := &v1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName(system.WorkflowExecutionPrefix+strings.TrimPrefix(step.Name, system.WorkflowStepPrefix), fmt.Sprintf("%d-%s", i, subCall.Workflow)),
				Namespace: step.Namespace,
			},
			Spec: v1.WorkflowExecutionSpec{
				Input:                 subCall.Input,
				ParentThreadName:      step.Status.ThreadName,
				ParentRunName:         step.Status.RunNames[i],
				WorkflowName:          wf.Name,
				AfterWorkflowStepName: step.Spec.AfterWorkflowStepName,
				WorkspaceName:         wf.Status.WorkspaceName,
				WorkflowGeneration:    step.Spec.WorkflowGeneration,
			},
		}

		if err := apply.New(req.Client).Apply(req.Ctx, req.Object, wfe); err != nil {
			return err
		}

		out, isErr, done, err := h.getSubflowOutput(req, wfe)
		if err != nil {
			return err
		}

		if isErr {
			step.Status.State = types.WorkflowStateError
			step.Status.Error = out
			return req.Client.Status().Update(req.Ctx, step)
		}

		if !done {
			return nil
		}

		resp, err := h.invoker.Step(req.Ctx, req.Client, step, invoke.StepOptions{
			PreviousRunName: step.Status.RunNames[i],
			Continue:        &out,
		})
		if err != nil {
			return err
		}
		defer resp.Close()

		step.Status.RunNames = append(step.Status.RunNames, resp.Run.Name)
		return req.Client.Status().Update(req.Ctx, step)
	}

	nextRunName := step.Status.RunNames[len(step.Status.SubCalls)]

	var run v1.Run
	if err := req.Get(&run, step.Namespace, nextRunName); err != nil {
		return err
	}

	switch run.Status.State {
	case gptscript.Continue, gptscript.Finished:
		if run.Status.SubCall != nil {
			step.Status.SubCalls = append(step.Status.SubCalls, *run.Status.SubCall)
		} else {
			step.Status.State = types.WorkflowStateComplete
			step.Status.LastRunName = nextRunName
			step.Status.Error = ""
		}
	case gptscript.Error:
		step.Status.State = types.WorkflowStateError
		step.Status.LastRunName = nextRunName
	}

	return nil
}

func (h *Handler) getSubflowOutput(req router.Request, wfe *v1.WorkflowExecution) (string, bool, bool, error) {
	var (
		check v1.WorkflowExecution
	)

	if err := req.Get(&check, wfe.Namespace, wfe.Name); apierrors.IsNotFound(err) {
		return "", false, false, nil
	} else if err != nil {
		return "", false, false, err
	}

	if check.Status.State == types.WorkflowStateError && check.Status.WorkflowGeneration == wfe.Spec.WorkflowGeneration {
		return check.Status.Error, true, true, nil
	}

	if check.Status.State != types.WorkflowStateComplete || check.Status.WorkflowGeneration != wfe.Spec.WorkflowGeneration {
		return "", false, false, nil
	}

	return check.Status.Output, false, true, nil
}
