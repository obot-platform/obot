package workflowstep

import (
	"fmt"
	"slices"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) RunWhile(req router.Request, resp router.Response) error {
	step := req.Object.(*v1.WorkflowStep)

	if step.Spec.Step.While == nil {
		return nil
	}

	count := step.Spec.Step.While.MaxLoops
	if count <= 0 {
		count = 3
	}

	var (
		finalState  = v1.WorkflowStepStateComplete
		lastRunName string
		lastStep    *v1.WorkflowStep
	)

	// Do one extra iteration to check the final state.
	count++
	for i := 0; i < count; i++ {
		if i == count-1 {
			finalState = v1.WorkflowStepStateError
			break
		}

		conditionStep := h.defineCondition(step, lastStep, fmt.Sprintf("while-%02d", i))
		resp.Objects(conditionStep)

		runName, conditionResult, wait, err := h.conditionResult(req.Ctx, req.Client, conditionStep)
		if err != nil {
			return err
		}
		lastRunName = runName

		if wait {
			finalState = v1.WorkflowStepStateRunning
			break
		}

		if !conditionResult {
			finalState = v1.WorkflowStepStateComplete
			break
		}

		steps, err := h.defineWhile(i, conditionStep, step)
		if err != nil {
			return err
		}

		if len(steps) > 0 {
			lastWfStep := steps[len(steps)-1].(*v1.WorkflowStep)
			lastStep = lastWfStep
		} else {
			lastStep = conditionStep
		}

		resp.Objects(steps...)

		runName, newState, err := getStateFromSteps(req.Ctx, req.Client, steps)
		if err != nil {
			return err
		}
		lastRunName = runName

		if newState != v1.WorkflowStepStateComplete {
			finalState = newState
			break
		}
	}

	step.Status.State = finalState
	step.Status.LastRunName = lastRunName
	return nil
}

func (h *Handler) defineWhile(groupIndex int, conditionStep, step *v1.WorkflowStep) (result []kclient.Object, _ error) {
	steps := step.Spec.Step.While.Steps

	var (
		lastStepName string
	)

	for i, loopStep := range steps {
		stepPath := append(step.Spec.Path, fmt.Sprint(groupIndex), fmt.Sprint(i))
		stepName := name.SafeHashConcatName(slices.Concat([]string{step.Spec.WorkflowExecutionName}, stepPath)...)
		afterStepName := conditionStep.Name
		if i > 0 {
			afterStepName = lastStepName
		}
		result = append(result, &v1.WorkflowStep{
			ObjectMeta: metav1.ObjectMeta{
				Name:      stepName,
				Namespace: step.Namespace,
			},
			Spec: v1.WorkflowStepSpec{
				ParentWorkflowStepName: step.Name,
				AfterWorkflowStepName:  afterStepName,
				Step:                   loopStep,
				Path:                   stepPath,
				WorkflowName:           step.Spec.WorkflowName,
				WorkflowExecutionName:  step.Spec.WorkflowExecutionName,
				ThreadName:             step.Spec.ThreadName,
			},
		})

		lastStepName = stepName
	}

	return result, nil
}
