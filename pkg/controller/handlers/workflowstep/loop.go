package workflowstep

import (
	"context"
	"fmt"
	"regexp"

	"github.com/gptscript-ai/datasets/pkg/dataset"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var datasetIDRegex = regexp.MustCompile(`gds://[a-z0-9]+`)

func (h *Handler) RunLoop(req router.Request, _ router.Response) (err error) {
	rootStep := req.Object.(*v1.WorkflowStep)

	if len(rootStep.Spec.Step.Loop) == 0 {
		return nil
	}

	var (
		completeResponse bool
		objects          []kclient.Object
	)
	defer func() {
		apply := apply.New(req.Client)
		if !completeResponse {
			apply.WithNoPrune()
		}
		if applyErr := apply.Apply(req.Ctx, req.Object, objects...); applyErr != nil && err == nil {
			err = applyErr
		}
	}()

	// reset
	rootStep.Status.Error = ""

	dataStep := defineDataStep(rootStep)
	objects = append(objects, dataStep)

	if _, errMsg, state, err := GetStateFromSteps(req.Ctx, req.Client, rootStep.Spec.WorkflowGeneration, dataStep); err != nil {
		return err
	} else if state.IsBlocked() {
		rootStep.Status.State = state
		rootStep.Status.Error = errMsg
		return nil
	}

	_, datasetID, wait, err := getDataStepResult(req.Ctx, req.Client, rootStep, dataStep)
	if err != nil {
		return err
	}

	if wait {
		rootStep.Status.State = types.WorkflowStateRunning
		return nil
	}

	workspaceID, err := getWorkspaceID(req.Ctx, req.Client, rootStep)
	if err != nil {
		return err
	}

	// We use the dataset package rather than making SDK calls because it is more direct and more performant.
	// All that the SDK calls do is call out to a daemon tool that runs the same library code that we are referencing here.
	datasetManager, err := dataset.NewManager(workspaceID)
	if err != nil {
		return err
	}

	dataset, err := datasetManager.GetDataset(req.Ctx, datasetID)
	if err != nil {
		return err
	}

	lastStepName := dataStep.Name
	for elementIndex, element := range dataset.GetAllElements() {
		steps, err := defineLoop(elementIndex, element.Contents, lastStepName, rootStep)
		if err != nil {
			return err
		}

		objects = append(objects, steps...)

		if len(steps) > 0 {
			lastStepName = steps[len(steps)-1].GetName()
		}
	}

	runName, errMsg, newState, err := GetStateFromSteps(req.Ctx, req.Client, rootStep.Spec.WorkflowGeneration, objects...)
	if err != nil {
		return err
	}

	if newState.IsBlocked() {
		rootStep.Status.State = newState
		rootStep.Status.Error = errMsg
		return nil
	}

	if newState != types.WorkflowStateComplete {
		rootStep.Status.State = newState
		return nil
	}

	completeResponse = true
	rootStep.Status.State = types.WorkflowStateComplete
	rootStep.Status.LastRunName = runName
	return nil
}

func defineLoop(elementIndex int, element string, dataStepName string, rootStep *v1.WorkflowStep) (result []kclient.Object, _ error) {
	var previousStepName string
	for i, s := range rootStep.Spec.Step.Loop {
		afterStepName := dataStepName
		if i > 0 {
			afterStepName = previousStepName
		} else {
			// For the very first step, we need to add the element to the prompt.
			s = elementPrompt(element, s)
		}

		newStep := NewStep(rootStep.Namespace, rootStep.Spec.WorkflowExecutionName, afterStepName, rootStep.Spec.WorkflowGeneration, types.Step{
			ID:   fmt.Sprintf("%s{element=%d}{step=%d}", rootStep.Spec.Step.ID, elementIndex, i),
			Step: s,
		})
		result = append(result, newStep)
		previousStepName = newStep.Name
	}

	return result, nil
}

func elementPrompt(element, prompt string) string {
	return fmt.Sprintf(`
	Based on the data, follow the instructions below.

	Data: %s

	Instructions: %s
	`, element, prompt)
}

func defineDataStep(rootStep *v1.WorkflowStep) *v1.WorkflowStep {
	return NewStep(rootStep.Namespace, rootStep.Spec.WorkflowExecutionName, rootStep.Spec.AfterWorkflowStepName, rootStep.Spec.WorkflowGeneration, types.Step{
		ID:   rootStep.Spec.Step.ID + "-loopdata",
		Step: dataPrompt(rootStep.Spec.Step.Step),
	})
}

func dataPrompt(description string) string {
	return fmt.Sprintf(`
	Based on the following description, find the data requested by the user:
	%q

	If the data is not already available in the chat history, call any tools you need in order to find it.
	You are looking for a dataset ID, which has the prefix gds://.
	If you found the dataset ID, return exactly the dataset ID (including the gds:// prefix) and nothing else.
	If you did not find it, simply return "false" without quotes and nothing else.
	`, description)
}

func getDataStepResult(ctx context.Context, client kclient.Client, parentStep *v1.WorkflowStep, dataStep *v1.WorkflowStep) (runName string, datasetID string, wait bool, err error) {
	var checkStep v1.WorkflowStep
	if err := client.Get(ctx, router.Key(dataStep.Namespace, dataStep.Name), &checkStep); apierrors.IsNotFound(err) {
		return "", "", true, nil
	} else if err != nil {
		return "", "", false, err
	}

	if checkStep.Status.State != types.WorkflowStateComplete || checkStep.Status.LastRunName == "" {
		return "", "", true, nil
	}

	var run v1.Run
	if err := client.Get(ctx, router.Key(dataStep.Namespace, checkStep.Status.LastRunName), &run); err != nil {
		return "", "", false, err
	}

	datasetID = getDatasetID(run.Status.Output)
	if datasetID == "" {
		parentStep.Status.Error = "no dataset ID found in output"
		parentStep.Status.State = types.WorkflowStateError
		return "", "", true, nil
	}

	return run.Name, datasetID, false, nil
}

func getDatasetID(output string) string {
	return datasetIDRegex.FindString(output)
}

func getWorkspaceID(ctx context.Context, client kclient.Client, step *v1.WorkflowStep) (string, error) {
	var workflowExecution v1.WorkflowExecution
	if err := client.Get(ctx, router.Key(step.Namespace, step.Spec.WorkflowExecutionName), &workflowExecution); err != nil {
		return "", err
	}

	var thread v1.Thread
	if err := client.Get(ctx, router.Key(step.Namespace, workflowExecution.Status.ThreadName), &thread); err != nil {
		return "", err
	}

	return thread.Status.WorkspaceID, nil
}
