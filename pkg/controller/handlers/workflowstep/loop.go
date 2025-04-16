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

func (h *Handler) RunLoop(req router.Request, _ router.Response) (err error) {
	step := req.Object.(*v1.WorkflowStep)

	if len(step.Spec.Step.Loop) == 0 {
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

	var (
		// lastStep    *v1.WorkflowStep
		lastRunName string
	)

	// reset
	step.Status.Error = ""

	dataStep := defineDataStep(step)
	objects = append(objects, dataStep)

	if _, errMsg, state, err := GetStateFromSteps(req.Ctx, req.Client, step.Spec.WorkflowGeneration, dataStep); err != nil {
		return err
	} else if state.IsBlocked() {
		step.Status.State = state
		step.Status.Error = errMsg
		return nil
	}

	runName, datasetID, wait, err := getDataStepResult(req.Ctx, req.Client, step, dataStep)
	if err != nil {
		return err
	}
	lastRunName = runName

	if wait {
		step.Status.State = types.WorkflowStateRunning
		return nil
	}

	workspaceID, err := getWorkspaceID(req.Ctx, req.Client, step)
	if err != nil {
		return err
	}

	datasetManager, err := dataset.NewManager(workspaceID)
	if err != nil {
		return err
	}

	dataset, err := datasetManager.GetDataset(req.Ctx, datasetID)
	if err != nil {
		return err
	}

	for _, element := range dataset.Elements {

	}

	step.Status.State = types.WorkflowStateComplete
	step.Status.LastRunName = lastRunName
	return nil
}

func defineDataStep(step *v1.WorkflowStep) *v1.WorkflowStep {
	return NewStep(step.Namespace, step.Spec.WorkflowExecutionName, step.Spec.AfterWorkflowStepName, step.Spec.WorkflowGeneration, types.Step{
		ID:   step.Spec.Step.ID + "-loopdata",
		Step: dataPrompt(step.Spec.Step.Step),
	})
}

func dataPrompt(description string) string {
	return fmt.Sprintf(`
	Based on the following description, find the data requested by the user:
	%q

	If the data is not already available in the chat history, call any tools you need in order to find it.
	You are looking for a dataset ID, which has the prefix gds://.
	If you found the dataset ID, return exactly the dataset ID (including the gds:// prefix) and nothing else.
	If you did not find it, simply return "false" and nothing else.
	`, description)
}

func getDataStepResult(ctx context.Context, client kclient.Client, step *v1.WorkflowStep, dataStep *v1.WorkflowStep) (runName string, datasetID string, wait bool, err error) {
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
		return run.Name, "", false, fmt.Errorf("no dataset ID found in output: %q", run.Status.Output)
	}

	return run.Name, datasetID, false, nil
}

func getDatasetID(output string) string {
	return regexp.MustCompile(`gds://[a-z0-9]+`).FindString(output)
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
