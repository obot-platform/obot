package workflowstep

import (
	"context"
	"regexp"
	"strings"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/invoke"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	"github.com/gptscript-ai/otto/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (h *Handler) SetRunning(req router.Request, resp router.Response) error {
	step := req.Object.(*v1.WorkflowStep)

	if step.Status.State == v1.WorkflowStepStateComplete || step.Status.State == v1.WorkflowStepStateError {
		resp.DisablePrune()
		return nil
	}

	if step.Spec.AfterWorkflowStepName != "" {
		var parent v1.WorkflowStep
		if err := req.Get(&parent, step.Namespace, step.Spec.AfterWorkflowStepName); err != nil {
			return kclient.IgnoreNotFound(err)
		}

		if parent.Status.State != v1.WorkflowStepStateComplete {
			return nil
		}
	}

	if step.Status.State != v1.WorkflowStepStateRunning && step.Status.State != v1.WorkflowStepStateSubCall {
		step.Status.State = v1.WorkflowStepStateRunning
		if err := req.Client.Status().Update(req.Ctx, step); err != nil {
			return kclient.IgnoreNotFound(err)
		}
	}

	return nil
}

func getStateFromSteps[T kclient.Object](ctx context.Context, client kclient.Client, steps []T) (string, v1.WorkflowStepState, error) {
	for i, obj := range steps {
		var (
			genericObj kclient.Object = obj
		)
		step := genericObj.(*v1.WorkflowStep).DeepCopy()
		if err := client.Get(ctx, kclient.ObjectKeyFromObject(step), step); apierrors.IsNotFound(err) {
			if i == 0 {
				return "", v1.WorkflowStepStatePending, nil
			}
			return "", v1.WorkflowStepStateRunning, nil
		} else if err != nil {
			return "", "", err
		}
		if step.Status.State == v1.WorkflowStepStateError {
			return "", v1.WorkflowStepStateError, nil
		}
		if i == len(steps)-1 && step.Status.State == v1.WorkflowStepStateComplete {
			return step.Status.LastRunName, v1.WorkflowStepStateComplete, nil
		}
	}

	return "", v1.WorkflowStepStateRunning, nil
}

func Running(handler router.Handler) router.Handler {
	return router.HandlerFunc(func(req router.Request, resp router.Response) error {
		if req.Object == nil {
			return nil
		}
		step := req.Object.(*v1.WorkflowStep)
		if step.Status.State == v1.WorkflowStepStateRunning || step.Status.State == v1.WorkflowStepStatePending {
			return handler.Handle(req, resp)
		}
		return nil
	})
}

var replaceRegexp = regexp.MustCompile(`[{},=]+`)

func NewStep(namespace, workflowExecutionName string, afterStepName string, step types.Step) *v1.WorkflowStep {
	if step.ID == "" {
		panic("step ID is required")
	}

	newID := replaceRegexp.ReplaceAllString(step.ID, "-")
	stepName := name.SafeConcatName(system.WorkflowStepPrefix+strings.TrimPrefix(workflowExecutionName, system.WorkflowExecutionPrefix), newID)
	stepName = strings.Trim(stepName, "-")
	stepName = strings.ReplaceAll(stepName, "--", "-")

	return &v1.WorkflowStep{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      stepName,
			Namespace: namespace,
		},
		Spec: v1.WorkflowStepSpec{
			AfterWorkflowStepName: afterStepName,
			Step:                  step,
			WorkflowExecutionName: workflowExecutionName,
		},
	}
}
