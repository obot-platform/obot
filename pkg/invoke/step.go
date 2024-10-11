package invoke

import (
	"context"
	"encoding/json"

	"github.com/acorn-io/baaah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/render"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type StepOptions struct {
	PreviousRunName string
	Continue        *string
}

func (i *Invoker) Step(ctx context.Context, c kclient.Client, step *v1.WorkflowStep, opt StepOptions) (*Response, error) {
	agent, err := i.toAgentFromStep(ctx, c, step)
	if err != nil {
		return nil, err
	}

	input, err := i.getInput(step)
	if err != nil {
		return nil, err
	}

	if opt.Continue != nil {
		input = *opt.Continue
	}

	var wfe v1.WorkflowExecution
	if err := c.Get(ctx, router.Key(step.Namespace, step.Spec.WorkflowExecutionName), &wfe); err != nil {
		return nil, err
	}

	return i.Agent(ctx, c, &agent, input, Options{
		ThreadName:            wfe.Status.ThreadName,
		ParentThreadName:      wfe.Spec.ParentThreadName,
		PreviousRunName:       opt.PreviousRunName,
		ForceNoResume:         opt.PreviousRunName == "",
		WorkflowName:          wfe.Spec.WorkflowName,
		WorkflowExecutionName: step.Spec.WorkflowExecutionName,
		WorkflowStepName:      step.Name,
		WorkflowStepID:        step.Spec.Step.ID,
	})
}

func (i *Invoker) toAgentFromStep(ctx context.Context, c kclient.Client, step *v1.WorkflowStep) (v1.Agent, error) {
	var (
		wf  v1.Workflow
		wfe v1.WorkflowExecution
	)
	if err := c.Get(ctx, router.Key(step.Namespace, step.Spec.WorkflowExecutionName), &wfe); err != nil {
		return v1.Agent{}, err
	}
	if err := c.Get(ctx, router.Key(step.Namespace, wfe.Spec.WorkflowName), &wf); err != nil {
		return v1.Agent{}, err
	}
	return i.toAgent(ctx, c, &wf, step, wfe.Spec.Input, *wfe.Status.WorkflowManifest)
}

func (i *Invoker) toAgent(ctx context.Context, c kclient.Client, wf *v1.Workflow, step *v1.WorkflowStep, input string, manifest types.WorkflowManifest) (v1.Agent, error) {
	agent, err := render.Workflow(ctx, c, wf, render.WorkflowOptions{
		ManifestOverride: &manifest,
		Step:             &step.Spec.Step,
		Input:            input,
	})
	if err != nil {
		return v1.Agent{}, err
	}
	return *agent, nil
}

func toStringArgs(args map[string]string) (string, error) {
	if args == nil {
		args = map[string]string{}
	}
	data, err := json.Marshal(args)
	return string(data), err
}

func (i *Invoker) getInput(step *v1.WorkflowStep) (string, error) {
	if step.Spec.Step.Template != nil && step.Spec.Step.Template.Name != "" {
		return toStringArgs(step.Spec.Step.Template.Args)
	} else if step.Spec.Step.Step != "" {
		return step.Spec.Step.Step, nil
	}
	return "", nil
}
