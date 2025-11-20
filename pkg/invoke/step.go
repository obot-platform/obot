package invoke

import (
	"context"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type StepOptions struct {
	PreviousRunName string
	IgnoreMCPErrors bool
}

func (i *Invoker) Step(ctx context.Context, mcpSessionManager *mcp.SessionManager, gptClient *gptscript.GPTScript, c kclient.WithWatch, step *v1.WorkflowStep, opt StepOptions) (*Response, error) {
	input, err := i.getInput(step)
	if err != nil {
		return nil, err
	}

	var wfe v1.WorkflowExecution
	if err := c.Get(ctx, router.Key(step.Namespace, step.Spec.WorkflowExecutionName), &wfe); err != nil {
		return nil, err
	}

	var thread v1.Thread
	if err := c.Get(ctx, router.Key(step.Namespace, wfe.Status.ThreadName), &thread); err != nil {
		return nil, err
	}

	var extraEnv []string
	if wfe.Spec.TaskBreakCrumb != "" {
		extraEnv = []string{"OBOT_TASK_BREAD_CRUMB=" + wfe.Spec.TaskBreakCrumb}
	}

	return i.Thread(ctx, mcpSessionManager, gptClient, c, &thread, input, Options{
		UserUID:               thread.Spec.UserID,
		WorkflowName:          wfe.Spec.WorkflowName,
		WorkflowStepName:      step.Name,
		WorkflowStepID:        step.Spec.Step.ID,
		WorkflowExecutionName: wfe.Name,
		PreviousRunName:       opt.PreviousRunName,
		ForceNoResume:         opt.PreviousRunName == "",
		IgnoreMCPErrors:       opt.IgnoreMCPErrors,
		ExtraEnv:              extraEnv,
	})
}

func (i *Invoker) getInput(step *v1.WorkflowStep) (string, error) {
	return step.Spec.Step.Step, nil
}
