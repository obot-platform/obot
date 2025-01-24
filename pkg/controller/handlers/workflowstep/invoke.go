package workflowstep

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) RunInvoke(req router.Request, _ router.Response) error {
	var (
		ctx         = req.Ctx
		client      = req.Client
		step        = req.Object.(*v1.WorkflowStep)
		lastRunName string
	)

	if step.Spec.Step.If != nil || step.Spec.Step.While != nil {
		return nil
	}

	if step.Spec.AfterWorkflowStepName != "" {
		var previousStep v1.WorkflowStep
		if err := client.Get(ctx, router.Key(step.Namespace, step.Spec.AfterWorkflowStepName), &previousStep); err != nil {
			return err
		}
		lastRunName = previousStep.Status.LastRunName
	}

	var run v1.Run
	if len(step.Status.RunNames) == 0 {
		invokeResp, err := h.invoker.Step(ctx, req.Client, step, invoke.StepOptions{
			PreviousRunName: lastRunName,
		})
		if err != nil {
			return err
		}
		defer invokeResp.Close()

		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			if err := client.Get(ctx, router.Key(step.Namespace, step.Name), untriggered.UncachedGet(step)); err != nil {
				return err
			}
			step.Status.ThreadName = invokeResp.Thread.Name
			step.Status.RunNames = []string{invokeResp.Run.Name}
			return client.Status().Update(ctx, step)
		})
		if err != nil {
			return err
		}

		run = *invokeResp.Run
	} else {
		if err := req.Get(&run, step.Namespace, step.Status.RunNames[0]); err != nil {
			return err
		}
	}

	return h.setStepStateFromRun(req, step, &run)
}

func (h *Handler) getResultFromNext(req router.Request, step *v1.WorkflowStep, run *v1.Run) error {
	if run.Status.TaskResult.NextRunName == "" {
		return nil
	}
	var nextRun v1.Run
	if err := req.Get(&nextRun, step.Namespace, run.Status.TaskResult.NextRunName); kclient.IgnoreNotFound(err) != nil {
		return err
	} else if apierrors.IsNotFound(err) {
		return nil
	}
	return h.setStepStateFromRun(req, step, &nextRun)
}

func (h *Handler) setStepStateFromRun(req router.Request, step *v1.WorkflowStep, run *v1.Run) error {
	switch run.Status.State {
	case gptscript.Finished:
		step.Status.State = types.WorkflowStateBlocked
		step.Status.LastRunName = step.Status.RunNames[0]
		step.Status.Error = "Aborted"
	case gptscript.Continue:
		if run.Status.SubCall != nil {
			step.Status.State = types.WorkflowStateSubCall
			step.Status.SubCalls = []v1.SubCall{*run.Status.SubCall}
		} else if run.Status.TaskResult != nil {
			return h.getResultFromNext(req, step, run)
		} else {
			step.Status.State = types.WorkflowStateComplete
			step.Status.LastRunName = step.Status.RunNames[0]
			step.Status.SubCalls = nil
		}
		step.Status.Error = ""
	case gptscript.Error:
		step.Status.State = types.WorkflowStateError
		step.Status.LastRunName = step.Status.RunNames[0]
		step.Status.Error = run.Status.Error
	}
	return nil
}
