package workflowstep

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) RunIf(req router.Request, _ router.Response) (err error) {
	step := req.Object.(*v1.WorkflowStep)

	if step.Spec.Step.If == nil {
		return nil
	}

	var completeResponse bool
	objects := []kclient.Object{}
	defer func() {
		apply := apply.New(req.Client)
		if !completeResponse {
			apply.WithNoPrune()
		}
		if applyErr := apply.Apply(req.Ctx, req.Object, objects...); applyErr != nil && err == nil {
			err = applyErr
		}
	}()

	conditionStep := h.defineCondition(step, nil, 0)
	objects = append(objects, conditionStep)

	if _, errorMsg, state, err := GetStateFromSteps(req.Ctx, req.Client, step.Spec.WorkflowGeneration, conditionStep); err != nil {
		return err
	} else if state.IsBlocked() {
		step.Status.State = state
		step.Status.Error = errorMsg
		return nil
	}

	conditionRunName, conditionResult, wait, err := getConditionResult(req.Ctx, req.Client, step, conditionStep)
	if err != nil {
		return err
	} else if wait {
		return nil
	}

	steps, err := h.defineIfSteps(step, conditionStep, conditionResult)
	if err != nil {
		return err
	}
	objects = append(objects, steps...)
	completeResponse = true

	if len(steps) == 0 {
		step.Status.State = types.WorkflowStateComplete
		step.Status.LastRunName = conditionRunName
		return nil
	}

	runName, errMsg, newState, err := GetStateFromSteps(req.Ctx, req.Client, step.Spec.WorkflowGeneration, steps...)
	if err != nil {
		return err
	}

	if newState.IsBlocked() {
		step.Status.State = newState
		step.Status.Error = errMsg
		return nil
	}

	step.Status.State = newState
	step.Status.LastRunName = runName
	return nil
}

// getConditionResult assumes the conditionStep is not in a IsBlocking() state already. Always check for IsBlocking() before calling this function.
func getConditionResult(ctx context.Context, c kclient.Client, parentStep, conditionStep *v1.WorkflowStep) (runName string, result, wait bool, err error) {
	var checkStep v1.WorkflowStep
	if err := c.Get(ctx, router.Key(conditionStep.Namespace, conditionStep.Name), &checkStep); apierrors.IsNotFound(err) {
		return "", false, true, nil
	} else if err != nil {
		return "", false, false, err
	}

	if checkStep.Status.State != types.WorkflowStateComplete || checkStep.Status.LastRunName == "" {
		return "", false, true, nil
	}

	var run v1.Run
	if err := c.Get(ctx, router.Key(conditionStep.Namespace, checkStep.Status.LastRunName), &run); err != nil {
		return "", false, false, err
	}

	if isTrue(run.Status.Output) {
		return run.Name, true, false, nil
	} else if isFalse(run.Status.Output) {
		return run.Name, false, false, nil
	}

	parentStep.Status.Error = fmt.Sprintf("Error evaluating condition: %s", run.Status.Output)
	parentStep.Status.State = types.WorkflowStateError

	return "", false, true, nil
}

func isTrue(s string) bool {
	check := truthyNormalize(s)
	return check == "true" ||
		check == "yes" ||
		check == "t" ||
		check == "y"
}

func truthyNormalize(s string) string {
	return strings.TrimSpace(strings.ToLower(strings.ReplaceAll(s, `"`, "")))
}

func isFalse(s string) bool {
	check := truthyNormalize(s)
	return check == "false" ||
		check == "no" ||
		check == "f" ||
		check == "n"
}

func toStepCondition(s string) string {
	//input := "STEP_CONDITION: " + step.Spec.Step.If.Condition
	input := `Respond with only the word TRUE if the following condition is true, or FALSE if false:\n` + s
	return input
}

func (h *Handler) defineCondition(step, afterStep *v1.WorkflowStep, iteration int) *v1.WorkflowStep {
	afterStepName := step.Spec.AfterWorkflowStepName
	if afterStep != nil {
		afterStepName = afterStep.Name
	}

	var (
		condition = "false"
		suffix    string
	)
	if step.Spec.Step.If != nil {
		condition = step.Spec.Step.If.Condition
		suffix = "{condition}"
	} else if step.Spec.Step.While != nil {
		condition = step.Spec.Step.While.Condition
		suffix = fmt.Sprintf("{condition,index=%d}", iteration)
	}

	newStep := NewStep(step.Namespace, step.Spec.WorkflowExecutionName, afterStepName, step.Spec.WorkflowGeneration, types.Step{
		ID:   step.Spec.Step.ID + suffix,
		Step: toStepCondition(condition),
	})
	return newStep
}

func (h *Handler) defineIfSteps(step *v1.WorkflowStep, conditionStep *v1.WorkflowStep, conditionResult bool) (result []kclient.Object, _ error) {
	var steps []types.Step
	if conditionResult {
		steps = step.Spec.Step.If.Steps
	} else {
		steps = step.Spec.Step.If.Else
	}

	var lastStepName string
	for i, ifStep := range steps {
		afterStepName := conditionStep.Name
		if i > 0 {
			afterStepName = lastStepName
		}
		newStep := NewStep(step.Namespace, step.Spec.WorkflowExecutionName, afterStepName, step.Spec.WorkflowGeneration, ifStep)
		result = append(result, newStep)
		lastStepName = newStep.Name
	}

	return result, nil
}
