package workflowstep

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/gptscript-ai/datasets/pkg/dataset"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/hash"
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

	fileName := hash.String(rootStep.Spec.Step.ID)[:8]
	dataStep := defineDataStep(rootStep, fileName)
	objects = append(objects, dataStep)

	if _, errMsg, state, err := GetStateFromSteps(req.Ctx, req.Client, rootStep.Spec.WorkflowGeneration, dataStep); err != nil {
		return err
	} else if state.IsBlocked() {
		rootStep.Status.State = state
		rootStep.Status.Error = errMsg
		return nil
	}

	workspaceID, data, wait, err := h.getDataStepResult(req.Ctx, req.Client, dataStep, fileName)
	if err != nil {
		return err
	}

	if wait {
		rootStep.Status.State = types.WorkflowStateRunning
		return nil
	}

	lastStepName := dataStep.Name
	for elementIndex, element := range data {
		steps, err := defineLoop(elementIndex, element, lastStepName, rootStep)
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

	// We ignore the error here because it does not really matter if we fail to delete the file.
	// We're just making a best effort to clean up after ourselves.
	_ = h.gptscriptClient.DeleteFileInWorkspace(req.Ctx, fileName, gptscript.DeleteFileInWorkspaceOptions{
		WorkspaceID: workspaceID,
	})
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

func defineDataStep(rootStep *v1.WorkflowStep, fileName string) *v1.WorkflowStep {
	return NewStep(rootStep.Namespace, rootStep.Spec.WorkflowExecutionName, rootStep.Spec.AfterWorkflowStepName, rootStep.Spec.WorkflowGeneration, types.Step{
		ID:   rootStep.Spec.Step.ID + "{loopdata}",
		Step: dataPrompt(rootStep.Spec.Step.Step, fileName),
	})
}

func dataPrompt(description, fileName string) string {
	return fmt.Sprintf(`
	Based on the following description, find the data requested by the user:
	%q

	If the data is not already available in the chat history, call any tools you need in order to find it.
	If the data includes a dataset ID (begins with gds://), call the loop-data tool with it as the dataset_id argument.
	If you do not find a dataset ID, create a JSON list of strings with the relevant data and call the loop-data tool with it as the data_list argument.

	When you call the loop-data tool, include %s as the file_name argument.
	`, description, fileName)
}

func (h *Handler) getDataStepResult(ctx context.Context, client kclient.Client, dataStep *v1.WorkflowStep, fileName string) (workspaceID string, data []string, wait bool, err error) {
	var checkStep v1.WorkflowStep
	if err := client.Get(ctx, router.Key(dataStep.Namespace, dataStep.Name), &checkStep); apierrors.IsNotFound(err) {
		return "", nil, true, nil
	} else if err != nil {
		return "", nil, false, err
	}

	if checkStep.Status.State != types.WorkflowStateComplete || checkStep.Status.LastRunName == "" {
		return "", nil, true, nil
	}

	var run v1.Run
	if err := client.Get(ctx, router.Key(checkStep.Namespace, checkStep.Status.LastRunName), &run); err != nil {
		return "", nil, false, err
	}

	var thread v1.Thread
	if err := client.Get(ctx, router.Key(run.Namespace, run.Spec.ThreadName), &thread); err != nil {
		return "", nil, false, err
	}

	content, err := h.gptscriptClient.ReadFileInWorkspace(ctx, fileName, gptscript.ReadFileInWorkspaceOptions{
		WorkspaceID: thread.Status.WorkspaceID,
	})
	if err != nil {
		return "", nil, false, err
	}

	if isDatasetID(content) {
		// We use the dataset package rather than making SDK calls because it is more direct and more performant.
		// All that the SDK calls do is call out to a daemon tool that runs the same library code that we are referencing here.
		datasetManager, err := dataset.NewManager(thread.Status.WorkspaceID)
		if err != nil {
			return "", nil, false, err
		}

		dataset, err := datasetManager.GetDataset(ctx, string(content))
		if err != nil {
			return "", nil, false, err
		}

		for _, element := range dataset.GetAllElements() {
			data = append(data, fmt.Sprintf("%s: %s", element.Name, element.Contents))
		}
	} else {
		if err := json.Unmarshal(content, &data); err != nil {
			return "", nil, false, err
		}
	}

	return thread.Status.WorkspaceID, data, false, nil
}

func isDatasetID(output []byte) bool {
	return datasetIDRegex.Match(output)
}
