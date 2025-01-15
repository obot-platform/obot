package workflow

import (
	"encoding/json"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetTaskResult(req router.Request, _ router.Response) error {
	run := req.Object.(*v1.Run)
	if run.Status.TaskResult == nil || run.Status.TaskResult.NextRunName != "" {
		return nil
	}

	var sourceThreadName string
	var ws v1.WorkflowStep
	if run.Spec.WorkflowStepName != "" {
		if err := req.Client.Get(req.Ctx, router.Key(run.Namespace, run.Spec.WorkflowStepName), &ws); err != nil {
			return kclient.IgnoreNotFound(err)
		}
		var wfe v1.WorkflowExecution
		if err := req.Client.Get(req.Ctx, router.Key(run.Namespace, ws.Spec.WorkflowExecutionName), &wfe); err != nil {
			return kclient.IgnoreNotFound(err)
		}
		var workflow v1.Workflow
		if err := req.Client.Get(req.Ctx, router.Key(run.Namespace, wfe.Spec.WorkflowName), &workflow); err != nil {
			return kclient.IgnoreNotFound(err)
		}
		sourceThreadName = wfe.Spec.ThreadName
	}

	if sourceThreadName == "" {
		sourceThreadName = run.Spec.ThreadName
	}

	var wfe v1.WorkflowExecution
	if err := req.Client.Get(req.Ctx, router.Key(run.Namespace, run.Status.TaskResult.ID), &wfe); err != nil {
		return kclient.IgnoreNotFound(err)
	}

	var workflow v1.Workflow
	if err := req.Client.Get(req.Ctx, router.Key(run.Namespace, wfe.Spec.WorkflowName), &workflow); err != nil {
		return kclient.IgnoreNotFound(err)
	}

	var output string
	if workflow.Spec.ThreadName != sourceThreadName {
		output = "ID: " + wfe.Name + " not found\n"
	} else if !wfe.Status.State.IsTerminal() {
		return nil
	} else {
		result, _ := json.Marshal(map[string]any{
			"state":  wfe.Status.State,
			"output": wfe.Status.Output,
		})
		output = string(result)
	}

	input, _ := json.Marshal(map[string]any{
		"return": output,
	})

	newRun := v1.Run{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.RunPrefix,
			Namespace:    run.Namespace,
		},
		Spec: *run.Spec.DeepCopy(),
	}
	newRun.Spec.PreviousRunName = run.Name
	newRun.Spec.Input = string(input)

	if err := req.Client.Create(req.Ctx, &newRun); err != nil {
		return err
	}

	run.Status.TaskResult.NextRunName = newRun.Name
	return req.Client.Update(req.Ctx, run)
}
