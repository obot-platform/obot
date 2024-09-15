package workflowstep

import (
	"encoding/json"
	"slices"

	"github.com/acorn-io/baaah/pkg/name"
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/invoke"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (h *Handler) toAgent(req router.Request, step *v1.WorkflowStep) (v1.Agent, error) {
	var (
		wf v1.Workflow
		we v1.WorkflowExecution
	)
	if err := req.Get(&wf, step.Namespace, step.Spec.WorkflowName); err != nil {
		return v1.Agent{}, err
	}
	if err := req.Get(&we, step.Namespace, step.Spec.WorkflowExecutionName); err != nil {
		return v1.Agent{}, err
	}
	agent := v1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: wf.Namespace,
		},
		Spec: v1.AgentSpec{
			Manifest: we.Status.WorkflowManifest.AgentManifest,
		},
		Status: v1.AgentStatus{
			Workspace:          wf.Status.Workspace,
			KnowledgeWorkspace: wf.Status.KnowledgeWorkspace,
		},
	}
	if step.Spec.Step.Cache != nil {
		agent.Spec.Manifest.Cache = step.Spec.Step.Cache
	}
	if step.Spec.Step.Temperature != nil {
		agent.Spec.Manifest.Temperature = step.Spec.Step.Temperature
	}
	return agent, nil
}

func (h *Handler) RunInvoke(req router.Request, resp router.Response) error {
	var (
		ctx         = req.Ctx
		client      = req.Client
		step        = req.Object.(*v1.WorkflowStep)
		lastRunName string
	)

	if step.Spec.Step.If != nil || step.Spec.Step.While != nil || step.Spec.SubFlow != nil {
		return nil
	}

	if step.Spec.AfterWorkflowStepName != "" {
		var previousStep v1.WorkflowStep
		if err := client.Get(ctx, router.Key(step.Namespace, step.Spec.AfterWorkflowStepName), &previousStep); err != nil {
			return err
		}
		lastRunName = previousStep.Status.LastRunName
		if lastRunName == "" {
			lastRunName = previousStep.Status.FirstRunName
		}
	}

	var run v1.Run
	if step.Status.FirstRunName == "" {
		agent, err := h.toAgent(req, step)
		if err != nil {
			return err
		}

		input, err := h.getInput(step)
		if err != nil {
			return err
		}

		invokeResp, err := h.invoker.Agent(ctx, req.Client, &agent, input, invoke.Options{
			Background:       true,
			ThreadName:       step.Spec.ThreadName,
			PreviousRunName:  lastRunName,
			WorkflowName:     step.Spec.WorkflowName,
			WorkflowStepName: step.Name,
		})
		if err != nil {
			return err
		}

		step.Status.ThreadName = invokeResp.Thread.Name
		step.Status.FirstRunName = invokeResp.Run.Name

		// Ignored error updating
		_ = client.Status().Update(ctx, step)
		invokeResp.Wait()

		run = *invokeResp.Run
	} else {
		if err := req.Get(&run, step.Namespace, step.Status.FirstRunName); err != nil {
			return err
		}
	}

	switch run.Status.State {
	case gptscript.Continue, gptscript.Finished:
		var err error
		step.Status.LastRunName, step.Status.State, err = h.processTailCall(req, resp, step, run.Status.Output)
		if err != nil {
			return err
		}
	case gptscript.Error:
		step.Status.State = v1.WorkflowStepStateError
		step.Status.Error = run.Status.Error
	}

	return nil
}

type call struct {
	Type     string `json:"type,omitempty"`
	Workflow string `json:"workflow,omitempty"`
	Input    any    `json:"input,omitempty"`
}

func (h *Handler) processTailCall(req router.Request, resp router.Response, step *v1.WorkflowStep, output string) (string, v1.WorkflowStepState, error) {
	var call call
	if err := json.Unmarshal([]byte(output), &call); err != nil || call.Type != "OttoSubFlow" || call.Workflow == "" {
		return step.Status.FirstRunName, v1.WorkflowStepStateComplete, nil
	}

	stepPath := append(step.Spec.Path, "subcall")
	stepName := name.SafeHashConcatName(slices.Concat([]string{step.Spec.WorkflowExecutionName}, stepPath)...)

	var inputString string
	switch v := call.Input.(type) {
	case string:
		inputString = v
	default:
		inputBytes, err := json.Marshal(v)
		if err != nil {
			return "", "", err
		}
		inputString = string(inputBytes)
	}

	if inputString == "{}" {
		inputString = ""
	}

	subCall := &v1.WorkflowStep{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stepName,
			Namespace: step.Namespace,
		},
		Spec: v1.WorkflowStepSpec{
			ParentWorkflowStepName: step.Name,
			AfterWorkflowStepName:  step.Spec.AfterWorkflowStepName,
			Step: v1.Step{
				Input: inputString,
			},
			SubFlow: &v1.SubFlow{
				Workflow: call.Workflow,
			},
			Path:                  stepPath,
			WorkflowName:          step.Spec.WorkflowName,
			WorkflowExecutionName: step.Spec.WorkflowExecutionName,
			ThreadName:            step.Spec.ThreadName,
		},
	}

	resp.Objects(subCall)

	var checkState v1.WorkflowStep
	if err := req.Get(&checkState, subCall.Namespace, subCall.Name); apierrors.IsNotFound(err) {
		return "", v1.WorkflowStepStateRunning, nil
	} else if err != nil {
		return "", "", err
	}

	if checkState.Status.State == v1.WorkflowStepStateError {
		return "", v1.WorkflowStepStateError, nil
	}

	if checkState.Status.State == v1.WorkflowStepStateComplete {
		return checkState.Status.LastRunName, v1.WorkflowStepStateComplete, nil
	}

	return "", v1.WorkflowStepStateRunning, nil
}
