package workflowstep

import (
	"fmt"

	"github.com/otto8-ai/nah/pkg/apply"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) RunWhile(req router.Request, _ router.Response) (err error) {
	step := req.Object.(*v1.WorkflowStep)

	if step.Spec.Step.While == nil {
		return nil
	}

	var completeResponse bool
	var objects []kclient.Object
	defer func() {
		apply := apply.New(req.Client)
		if !completeResponse {
			apply.WithNoPrune()
		}
		if applyErr := apply.Apply(req.Ctx, req.Object, objects...); applyErr != nil && err == nil {
			err = applyErr
		}
	}()

	count := step.Spec.Step.While.MaxLoops
	if count <= 0 {
		count = 3
	}

	var (
		lastStep    *v1.WorkflowStep
		lastRunName string
	)

	// reset
	step.Status.Error = ""

	// Do one extra iteration to check the final state.
	count++
	for i := 0; i < count; i++ {
		if i == count-1 {
			step.Status.State = types.WorkflowStateError
			step.Status.Error = fmt.Sprintf("MaxLoops exceeded count %d", count-1)
			return nil
		}

		conditionStep := h.defineCondition(step, lastStep, i)
		objects = append(objects, conditionStep)

		if _, errMsg, state, err := GetStateFromSteps(req.Ctx, req.Client, step.Spec.WorkflowGeneration, conditionStep); err != nil {
			return err
		} else if state.IsBlocked() {
			step.Status.State = state
			step.Status.Error = errMsg
			return nil
		}

		runName, conditionResult, wait, err := getConditionResult(req.Ctx, req.Client, step, conditionStep)
		if err != nil {
			return err
		}
		lastRunName = runName

		if wait {
			step.Status.State = types.WorkflowStateRunning
			return nil
		}

		if !conditionResult {
			completeResponse = true
			step.Status.State = types.WorkflowStateComplete
			step.Status.LastRunName = lastRunName
			return nil
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

		objects = append(objects, steps...)

		runName, errMsg, newState, err := GetStateFromSteps(req.Ctx, req.Client, step.Spec.WorkflowGeneration, steps...)
		if err != nil {
			return err
		}
		lastRunName = runName

		if newState.IsBlocked() {
			step.Status.State = newState
			step.Status.Error = errMsg
			return nil
		}

		if newState != types.WorkflowStateComplete {
			step.Status.State = newState
			return nil
		}
	}

	completeResponse = true
	step.Status.State = types.WorkflowStateComplete
	step.Status.LastRunName = lastRunName
	return nil
}

func (h *Handler) defineWhile(groupIndex int, conditionStep, step *v1.WorkflowStep) (result []kclient.Object, _ error) {
	steps := step.Spec.Step.While.Steps

	var (
		lastStepName string
	)

	for i, loopStep := range steps {
		afterStepName := conditionStep.Name
		if i > 0 {
			afterStepName = lastStepName
		}
		loopStep.ID = fmt.Sprintf("%s{index=%d}", loopStep.ID, groupIndex)
		newStep := NewStep(step.Namespace, step.Spec.WorkflowExecutionName, afterStepName, step.Spec.WorkflowGeneration, loopStep)
		result = append(result, newStep)
		lastStepName = newStep.Name
	}

	return result, nil
}
